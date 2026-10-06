package resourcelifetime

import (
	"go/constant"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Resource contracts are the authoritative acquisition, cleanup, and transfer
// vocabulary for this analyzer. Exact symbols are required so similarly named
// application methods do not imply ownership.

// resourceFamily selects the lifecycle policy for a proven API contract.
// Unknown and inferred owners cannot borrow a standard-library cleanup policy.
type resourceFamily uint8

const (
	resourceFamilyUnknown resourceFamily = iota
	resourceFamilyOS
	resourceFamilySQL
	resourceFamilyHTTP
	resourceFamilyCompression
	resourceFamilyOwned
)

type resourceContract struct {
	symbol      syntax.Symbol
	family      resourceFamily
	packagePath string
	name        string
	cleanup     []string
	result      int
	// retained marks a wrapper result that holds a resource no method of
	// the wrapper can release; the diagnostic says so.
	retained bool
	// role names which result this contract owns, when the call returns
	// more than one resource, as os.Pipe returns a read end and a write end.
	role string
}

func resourceContracts() []resourceContract {
	return []resourceContract{
		resourceFunction(resourceFamilyOS, "os", "Create", 0, "Close"),
		resourceFunction(resourceFamilyOS, "os", "CreateTemp", 0, "Close"),
		resourceFunction(resourceFamilyOS, "os", "Open", 0, "Close"),
		resourceFunction(resourceFamilyOS, "os", "OpenFile", 0, "Close"),
		// Each end of a pipe is its own descriptor: closing one releases
		// nothing of the other.
		resourceFunction(resourceFamilyOS, "os", "Pipe", 0, "Close").withRole("read end"),
		resourceFunction(resourceFamilyOS, "os", "Pipe", 1, "Close").withRole("write end"),
		// Channel timers are GC-managed since Go 1.23. Missing Stop alone
		// proves no leak. Main-module/runtime overrides are not established by
		// this package-local pass, so legacy timer behavior is not inferred.
		// This says nothing about AfterFunc callbacks or retained workers.
		// https://github.com/okteto/okteto/blob/ad42c0823762a2255d4b4ad2e53fb4ec190010e7/cmd/deploy/wait.go#L70-L71

		resourceMethod(resourceFamilySQL, "database/sql", "DB", "Begin", "Commit", "Rollback"),
		resourceMethod(resourceFamilySQL, "database/sql", "DB", "BeginTx", "Commit", "Rollback"),
		resourceMethod(resourceFamilySQL, "database/sql", "Conn", "BeginTx", "Commit", "Rollback"),
		resourceMethod(resourceFamilySQL, "database/sql", "DB", "Query", "Close"),
		resourceMethod(resourceFamilySQL, "database/sql", "DB", "QueryContext", "Close"),
		resourceMethod(resourceFamilySQL, "database/sql", "Conn", "QueryContext", "Close"),
		resourceMethod(resourceFamilySQL, "database/sql", "Tx", "Query", "Close"),
		resourceMethod(resourceFamilySQL, "database/sql", "Tx", "QueryContext", "Close"),
		resourceMethod(resourceFamilySQL, "database/sql", "Stmt", "Query", "Close"),
		resourceMethod(resourceFamilySQL, "database/sql", "Stmt", "QueryContext", "Close"),
		// Statements prepared on a transaction are closed automatically when that
		// transaction commits or rolls back, so Tx.Prepare* is deliberately absent.
		resourceMethod(resourceFamilySQL, "database/sql", "DB", "Prepare", "Close"),
		resourceMethod(resourceFamilySQL, "database/sql", "DB", "PrepareContext", "Close"),
		resourceMethod(resourceFamilySQL, "database/sql", "Conn", "PrepareContext", "Close"),

		resourceFunction(resourceFamilyHTTP, "net/http", "Get", 0, "Close"),
		resourceFunction(resourceFamilyHTTP, "net/http", "Post", 0, "Close"),
		resourceFunction(resourceFamilyHTTP, "net/http", "PostForm", 0, "Close"),
		resourceMethod(resourceFamilyHTTP, "net/http", "Client", "Do", "Close"),
		// net/http documents the same obligation for these Client methods as
		// for the package functions above: "Caller should close resp.Body when
		// done reading from it." Head carries no such sentence, and a HEAD
		// response usually has http.NoBody, so neither Head form is listed.
		resourceMethod(resourceFamilyHTTP, "net/http", "Client", "Get", "Close"),
		resourceMethod(resourceFamilyHTTP, "net/http", "Client", "Post", "Close"),
		resourceMethod(resourceFamilyHTTP, "net/http", "Client", "PostForm", "Close"),

		// Compression readers do not own their inputs or require finalization.
		// Close neither closes the underlying reader nor validates a checksum;
		// missing it is not a resource-lifetime violation.
		// https://github.com/flux-iac/tofu-controller/blob/8fc67730e8b5d48092060f22b89bd2e88fc0ce86/api/plan/gzip.go#L28
		resourceFunction(resourceFamilyCompression, "compress/gzip", "NewWriterLevel", 0, "Close"),
		resourceFunction(resourceFamilyCompression, "compress/gzip", "NewWriter", -1, "Close"),
		resourceFunction(resourceFamilyCompression, "compress/zlib", "NewWriterLevel", 0, "Close"),
		resourceFunction(resourceFamilyCompression, "compress/zlib", "NewWriterLevelDict", 0, "Close"),
		resourceFunction(resourceFamilyCompression, "compress/zlib", "NewWriter", -1, "Close"),
	}
}

func resourceFunction(family resourceFamily, packagePath, name string, result int, cleanup ...string) resourceContract {
	return resourceContract{
		symbol: syntax.PackageFunction(packagePath, name), family: family, packagePath: packagePath, name: name, cleanup: cleanup, result: result,
	}
}

func (contract resourceContract) withRole(role string) resourceContract {
	contract.role = role
	return contract
}

func resourceMethod(family resourceFamily, packagePath, receiver, name string, cleanup ...string) resourceContract {
	return resourceContract{
		symbol:      syntax.PackageMethod(syntax.MethodSymbol{PackagePath: packagePath, Receiver: receiver, Name: name}),
		family:      family,
		packagePath: packagePath,
		name:        name,
		cleanup:     cleanup,
		result:      0,
	}
}

func sqlDatabaseCall(common *ssa.CallCommon, names ...string) bool {
	return sqlReceiverCall(common, "DB", names...)
}

func sqlReceiverCall(common *ssa.CallCommon, receiver string, names ...string) bool {
	for _, name := range names {
		if ssacall.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
			PackagePath: "database/sql", Receiver: receiver, Name: name,
		})) {
			return true
		}
	}
	return false
}

// resourceContractsFor returns every contract the call matches: one for
// each owned result.
func resourceContractsFor(common *ssa.CallCommon, settings resourceLifetimeSettings) []resourceContract {
	var contracts []resourceContract
	for _, contract := range settings.catalog {
		if ssacall.CallMatchesSymbol(common, contract.symbol) {
			contracts = append(contracts, contract)
		}
	}
	return contracts
}

// ownedResultContract synthesizes a contract for a call whose callee is
// summarized as returning a struct that owns resource fields and whose result
// type has a method releasing them. The caller then owes that method exactly
// as it owes Close to os.Open; the summaries are proven from the constructor
// and method bodies, so no name enters the decision. A constructor that
// stores a caller's resource, or a type whose methods never release the
// field, produces no contract.
func ownedResultContract(evidence *lifecyclefacts.LifecycleEvidence, call *ssa.Call, settings resourceLifetimeSettings) (resourceContract, bool) {
	callee := call.Common().StaticCallee()
	cleanup, index, ok := evidence.OwnedResult(call)
	if !ok && !catalogCoversPackage(settings, callee) {
		// A constructor may hand the resource back directly rather than in
		// a struct; the summary then names the result position and its type
		// names the cleanup. The catalog stays authoritative for the packages
		// it models: database/sql's Prepare returns a fresh statement by its
		// body, but the catalog deliberately leaves statements to the
		// transaction or database that owns them, and an inferred owner must
		// not reopen that decision.
		cleanup, index, ok = evidence.OwnedDirectResult(call)
	}
	retained := false
	if !ok && !catalogCoversPackage(settings, callee) {
		// A constructor may instead return a wrapper that holds the resource
		// it acquired. No method releases it, so the caller settles the
		// obligation only by keeping, handing over, or returning the wrapper.
		index, ok = evidence.RetainingResult(call)
		retained = ok
	}
	if !ok {
		return resourceContract{}, false
	}
	// The package name only labels the diagnostic; identity was decided by
	// the imported summaries above.
	return resourceContract{
		family:      resourceFamilyOwned,
		packagePath: callee.Pkg.Pkg.Name(),
		name:        callee.Name(),
		cleanup:     cleanup,
		result:      index,
		retained:    retained,
	}, true
}

// catalogCoversPackage reports whether any catalog contract names the
// callee's package, in which case the catalog's decisions about that API
// are complete and no inferred acquisition is added beside them.
func catalogCoversPackage(settings resourceLifetimeSettings, callee *ssa.Function) bool {
	if callee == nil || callee.Object() == nil {
		return false
	}
	for _, contract := range settings.catalog {
		if syntax.DeclaredInPackage(callee.Object(), contract.packagePath) {
			return true
		}
	}
	return false
}

// memoryWriterExempt reports whether a compression writer wraps a local
// in-memory buffer. Leaving such a writer unclosed on an error path holds no
// resource outside the function. A writer never closed before its buffer is
// read produces truncated output, which is a data defect rather than a leak
// and is not this check's claim.
func memoryWriterExempt(call *ssa.Call, contract resourceContract) bool {
	if contract.family != resourceFamilyCompression || len(call.Common().Args) == 0 {
		return false
	}
	writerMethods := contract.cleanup
	if len(writerMethods) == 0 {
		return false
	}
	underlying := call.Common().Args[0]
	if inner, ok := ssaflow.UnwrapTransparentValue(
		underlying,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		underlying = inner
	}
	// Standard constructors create the same memory-only buffer as a local
	// allocation. The input slice may be borrowed, but carries no descriptor
	// cleanup obligation. Dynamic factories and mixed external writers remain
	// outside this exemption.
	// https://github.com/apache/pulsar-client-go/blob/1a6d7ac818c9daae9df5c37cb24c0695fabc9eec/pulsar/internal/compression/zlib.go#L39-L52
	if constructor, ok := underlying.(*ssa.Call); ok && constructor.Parent() == call.Parent() {
		return ssacall.CallMatchesAnySymbol(constructor.Common(),
			syntax.PackageFunction("bytes", "NewBuffer"), syntax.PackageFunction("bytes", "NewBufferString"))
	}
	local, ok := underlying.(*ssa.Alloc)
	if !ok || local.Parent() != call.Parent() {
		return false
	}
	pointer, ok := local.Type().Underlying().(*types.Pointer)
	return ok && (syntax.NamedType(pointer.Elem(), "bytes", "Buffer") || syntax.NamedType(pointer.Elem(), "strings", "Builder"))
}

// HTTP acquisition contracts distinguish known body-bearing operations from
// exact local protocol shapes that may acquire no body. They feed the ordinary
// resource flow; no separate cleanup or reporting decision is made here.

func httpAcquisitionBoundary(pass *analysis.Pass, call *ssa.Call, budget *proofs.SearchBudget) resourceLifetimeReason {
	head := proveHeadAcquisitionWithin(call, budget)
	if head.State == proofs.EvidenceUnknown && head.Reason != resourceReasonNone {
		return head.Reason
	}
	if head.Reason != resourceReasonNone {
		// The rule applied to a Client.Do and declined; say which input failed
		// so a trace of the site does not need the source to explain it.
		probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos())
		if probe.Enabled() {
			probe.Considered(analysisTrace.Step{
				Reason: head.Reason.String(), Outcome: analysisTrace.OutcomeRejected, Pos: call.Pos(), Function: call.Parent().String(),
			})
		}
	}
	proof := proveLocalHeaderOnlyAcquisitionWithin(call, budget)
	if proof.Reason != resourceReasonNone {
		probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos())
		if probe.Enabled() {
			outcome := analysisTrace.OutcomeUnknown
			if proof.Proven() {
				outcome = analysisTrace.OutcomeAccepted
			}
			probe.Evidence(analysisTrace.Step{
				Reason: proof.Reason.String(), Outcome: outcome, Pos: call.Pos(), Function: call.Parent().String(),
			})
		}
	}
	if proof.State == proofs.EvidenceUnknown && proof.Reason == resourceReasonBudgetExhausted {
		return proof.Reason
	}
	if proof.Proven() {
		return resourceReasonHeaderOnlyAcquisition
	}
	return resourceReasonNone
}

// A HEAD response through an unconfigured client normally carries
// http.NoBody, not an acquired body: the standard transport reads no body for
// HEAD, and without Client.Timeout there is no cancelTimerBody around it.
// DefaultTransport and DefaultClient are replaceable, so this is uncertainty
// about acquisition, never a proof that closing is unnecessary. The client is
// either a fresh zero-value local used only by Do, or the package default
// client with no visible reconfiguration in this function or a visible callee;
// hidden cross-package mutation of the defaults is the same accepted coverage
// gap as the local-server boundary below. The request must be the direct
// HEAD constructor result, possibly rebound through WithContext or Clone and
// with its Header map edited, none of which can change Method. Any other use
// of the request or the client, such as a helper receiving it, could.
// https://github.com/vishen/go-chromecast/blob/5dd70bb91787fe28e3d8946682c66cb2a1d61d21/application/application.go#L723-L732
// https://github.com/alexellis/arkade/blob/0a0a800fd7554d4eddb1856f9ef8a21214e95bab/pkg/get/get.go#L236-L244
// https://github.com/deweizhu/bookget/blob/2cdbf6d6c3ce70355a5c4411c0faf3450e9ae877/pkg/downloader/downloader.go#L510-L522
func proveHeadAcquisitionWithin(call *ssa.Call, budget *proofs.SearchBudget) resourceProof {
	return findHeadAcquisitionWithin(call, budget).within(budget)
}

func findHeadAcquisitionWithin(call *ssa.Call, budget *proofs.SearchBudget) resourceProof {
	common := call.Common()
	if !ssacall.CallMatchesSymbol(common, httpClientDo) || len(common.Args) != 2 {
		return resourceProof{}
	}
	// A request that never came from a HEAD constructor is outside this rule,
	// so it stays silent; the rule explains itself only when it applied.
	if !headConstructedWithin(common.Args[1], budget) {
		return resourceProof{}
	}
	if !headRequestWithin(common.Args[1], budget) {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonHeadRequestModified}
	}
	client := proveHeadClientUnconfiguredWithin(common.Args[0], call.Parent(), budget)
	if client.State == proofs.EvidenceUnknown {
		return client
	}
	if !client.Proven() {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonHeadClientNotUnconfigured}
	}
	return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonHeadAcquisition}
}

var (
	httpClientDo           = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Client", Name: "Do"})
	httpDefaultClient      = syntax.PackageVariable("net/http", "DefaultClient")
	httpDefaultTransport   = syntax.PackageVariable("net/http", "DefaultTransport")
	httpRequestWithContext = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Request", Name: "WithContext"})
	httpRequestClone       = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Request", Name: "Clone"})
	// Header edits through the standard map methods cannot change the request
	// method; the map handed anywhere else is rejected so the rule stays small.
	httpHeaderEdits = []syntax.Symbol{
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Set"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Add"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Get"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Del"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Values"}),
	}
)

func constantString(value ssa.Value) string {
	text, ok := value.(*ssa.Const)
	if !ok || text.Value == nil || text.Value.Kind() != constant.String {
		return ""
	}
	return constant.StringVal(text.Value)
}
