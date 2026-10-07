package heapmodel

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const projectionBoundaryFixture = `
package ssaflowtest

type closer struct{}
func (*closer) Close() {}
type owner struct { body *closer }
type ownerView owner
type slotView **closer

func acquire() *owner { return nil }
func cleanup(*closer) {}
func mutateOwner(*owner)
func mutateSlot(**closer)
func inspectOwner(p *owner) *closer { return p.body }
func inspectSlot(p **closer) bool { return *p != nil }
func inspectView(p *ownerView) *closer { return p.body }
func mutateView(*ownerView)
func mutateSlotView(slotView)
var retained *owner
func retainOwner(p *owner) { retained=p }

func accepted() {
	value := acquire()
	cleanup(value.body)
}
func readOnlyRoot() {
	value := acquire()
	inspectOwner(value)
	cleanup(value.body)
}
func readOnlySlot() {
	value := acquire()
	inspectSlot(&value.body)
	cleanup(value.body)
}
func convertedReadOnlyRoot() {
	value := acquire()
	inspectView((*ownerView)(value))
	cleanup(value.body)
}
func convertedEscapedRoot() {
	value := acquire()
	mutateView((*ownerView)(value))
	cleanup(value.body)
}
func convertedEscapedSlot() {
	value := acquire()
	mutateSlotView(slotView(&value.body))
	cleanup(value.body)
}
func retainedRoot() {
	value := acquire()
	retainOwner(value)
	cleanup(value.body)
}
func escapedLater() {
	value := acquire()
	cleanup(value.body)
	mutateOwner(value)
}
func reassigned() {
	value := acquire()
	value.body = &closer{}
	cleanup(value.body)
}
func escapedRoot() {
	value := acquire()
	mutateOwner(value)
	cleanup(value.body)
}
func escapedAddress() {
	value := acquire()
	mutateSlot(&value.body)
	cleanup(value.body)
}
func selectedOwner(choose bool) {
	value := acquire()
	other := acquire()
	selected := other
	if choose { selected = value }
	cleanup(selected.body)
}
func sibling() {
	value := acquire()
	other := acquire()
	_ = value
	cleanup(other.body)
}
`

func TestUnmodifiedNonEmptyAccessPathAtBoundaries(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "example.com/ssaflowtest", projectionBoundaryFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "accepted", want: true},
		{name: "readOnlyRoot", want: true},
		{name: "readOnlySlot", want: true},
		{name: "convertedReadOnlyRoot", want: true},
		{name: "convertedEscapedRoot"},
		{name: "convertedEscapedSlot"},
		{name: "retainedRoot"},
		{name: "escapedLater", want: true},
		{name: "reassigned"},
		{name: "escapedRoot"},
		{name: "escapedAddress"},
		{name: "selectedOwner"},
		{name: "sibling"},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			if strings.HasPrefix(test.name, "converted") && len(ssaflow.InstructionsOf[*ssa.ChangeType](function)) == 0 {
				t.Fatal("fixture did not retain a conversion wrapper")
			}
			var root ssa.Value
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					common := ssaflow.InstructionCall(instruction)
					if common != nil && ssaflow.CallName(common) == "acquire" && root == nil {
						root, _ = instruction.(ssa.Value)
					}
				}
			}
			var cleanupCall *ssa.Call
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
				if ssaflow.CallName(call.Common()) == "cleanup" {
					cleanupCall = call
					break
				}
			}
			if cleanupCall == nil {
				t.Fatal("missing cleanup call")
			}
			argument := ssaflow.InstructionCall(cleanupCall).Args[0]
			if got := NewStorage(proofs.NewSearchBudget(1000)).Projection(argument, root, cleanupCall).Proven(); got != test.want {
				t.Fatalf("UnmodifiedNonEmptyAccessPathAt() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestProjectionCutoffPreservesMutationBoundary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "example.com/ssaflowtest", projectionBoundaryFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"accepted", true},
		{"readOnlyRoot", true},
		{"readOnlySlot", true},
		{"convertedReadOnlyRoot", true},
		{"escapedLater", true},
		{"convertedEscapedRoot", false},
		{"convertedEscapedSlot", false},
		{"reassigned", false},
		{"escapedRoot", false},
		{"escapedAddress", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, observation := projectionQuery(t, pkg.Func(test.name))
			argument := observation.Common().Args[0]
			for allowance := 1; allowance <= proofs.QueryBudget; allowance++ {
				budget := proofs.NewSearchBudget(allowance)
				proof := NewStorage(budget).Projection(argument, root, observation)
				if budget.Exhausted() {
					if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("cut %d published evidence: %+v", allowance, proof)
					}
					continue
				}
				if proof.Proven() != test.want {
					t.Fatalf("complete %d: %+v, want proven=%v", allowance, proof, test.want)
				}
				return
			}
			t.Fatal("small query never completed")
		})
	}
}

func TestProjectionRetainsIndependentPathCutoff(t *testing.T) {
	source := `package projectionprobe
 type node struct { child *node; body *int }
 func acquire() *node { return nil }
 func cleanup(*int){}
 func deep(){p:=acquire(); cleanup(p.` + strings.Repeat("child.", proofs.QueryBudget+1) + `body)}
 func shallow(){p:=acquire(); cleanup(p.body)}
 `
	pkg := ssaflowtest.BuildPackage(t, "projectionprobe", source)
	budget := proofs.NewSearchBudget(10 * proofs.QueryBudget)
	storage := NewStorage(budget)
	root, observation := projectionQuery(t, pkg.Func("deep"))
	proof := storage.Projection(observation.Common().Args[0], root, observation)
	if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted || budget.Exhausted() {
		t.Fatalf("child cutoff lost: %+v, parent exhausted=%v", proof, budget.Exhausted())
	}
	root, observation = projectionQuery(t, pkg.Func("shallow"))
	if proof := storage.Projection(observation.Common().Args[0], root, observation); !proof.Proven() {
		t.Fatalf("independent cutoff poisoned a fresh query: %+v", proof)
	}
}

func TestProjectionObservationWindowCutoffIsUnavailable(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "example.com/ssaflowtest", projectionBoundaryFixture)
	fn := pkg.Func("reassigned")
	root, observation := projectionQuery(t, fn)
	origin := root.(ssa.Instruction)
	stores := ssaflow.InstructionsOf[*ssa.Store](fn)
	if len(stores) != 1 {
		t.Fatal("fixture lost selected field replacement")
	}
	cut := proofs.NewSearchBudget(1)
	if instructionWithinObservation(stores[0], origin, observation, cut) || !cut.Exhausted() {
		t.Fatal("observation window bypassed caller allowance")
	}
	if !instructionWithinObservation(stores[0], origin, observation, proofs.NewSearchBudget(proofs.QueryBudget)) {
		t.Fatal("fresh query lost mutating use inside observation window")
	}
	cut = proofs.NewSearchBudget(1)
	if NewStorage(cut).addressDoesNotEscapeBetween(stores[0].Addr, origin, observation, map[ssa.Value]bool{}) || !cut.Exhausted() {
		t.Fatal("shortened observation window certified stable storage")
	}
}

func TestEmbeddedFieldSetupSharesStorageAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "fieldprobe", `package fieldprobe
 type inner struct { body *int }
 type owner struct { nested inner; sibling int }
 func observe(*owner){}
 func stable(){p:=new(owner); p.nested.body=new(int); p.sibling++; observe(p)}
 func changed(){p:=new(owner); p.nested.body=new(int); observe(p); p.nested.body=nil}
 `)
	for _, name := range []string{"stable", "changed"} {
		fn := pkg.Func(name)
		var address ssa.Value
		for _, field := range ssaflow.InstructionsOf[*ssa.FieldAddr](fn) {
			if _, nested := field.X.(*ssa.FieldAddr); nested {
				address = field
				break
			}
		}
		observation := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
		if address == nil {
			t.Fatal("fixture lost nested field address")
		}
		assertEmbeddedFieldSetupCutoff(t, address)
		completed := false
		for allowance := 1; allowance <= proofs.QueryBudget; allowance++ {
			budget := proofs.NewSearchBudget(allowance)
			proof := NewStorage(budget).StableFieldContent(address, observation)
			if budget.Exhausted() {
				if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
					t.Fatalf("%s cut %d: %+v", name, allowance, proof)
				}
				continue
			}
			if proof.Proven() != (name == "stable") {
				t.Fatalf("%s complete %d: %+v", name, allowance, proof)
			}
			completed = true
			break
		}
		if !completed {
			t.Fatal("small embedded field query never completed")
		}
	}
}

func projectionQuery(t *testing.T, fn *ssa.Function) (ssa.Value, *ssa.Call) {
	t.Helper()
	var root ssa.Value
	var observation *ssa.Call
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		switch ssaflow.CallName(call.Common()) {
		case "acquire":
			if root == nil {
				root = call
			}
		case "cleanup":
			observation = call
		}
	}
	if root == nil || observation == nil {
		t.Fatal("fixture lost acquisition or observation")
	}
	return root, observation
}

func assertEmbeddedFieldSetupCutoff(t *testing.T, address ssa.Value) {
	t.Helper()
	cut := proofs.NewSearchBudget(1)
	path, known := ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(cut), address,
		func(value ssa.Value) bool { _, fresh := value.(*ssa.Alloc); return fresh })
	if known || !cut.Exhausted() || path.Depth != 0 {
		t.Fatalf("fixture did not cut during embedded setup: %+v, known=%v", path, known)
	}
	// Three visits suffice to name this allocation/field/field location.
	// Embedded setup must share them even when no observation is supplied.
	setup := proofs.NewSearchBudget(3)
	if proof := NewStorage(setup).StableFieldContent(address, nil); !setup.Exhausted() || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("embedded setup bypassed storage allowance: %+v", proof)
	}
}

func TestOrderedSlotsPreservesPublicationOrder(t *testing.T) {
	first := &region{serial: 1}
	second := &region{serial: 2}
	want := []slot{{region: first, path: "field:10"}, {region: first, path: "field:2"}, {region: second}}
	entries := map[slot]bool{want[2]: true, want[1]: false, want[0]: true}
	before := maps.Clone(entries)
	if got := orderedSlots(entries); !slices.Equal(got, want) {
		t.Fatalf("ordered slots = %v, want %v", got, want)
	}
	if !maps.Equal(entries, before) {
		t.Fatal("sorting changed input evidence")
	}
	if got := orderedSlots(map[slot]bool(nil)); got == nil || len(got) != 0 {
		t.Fatal("empty ordering must return a nonnil empty slice")
	}
}

func TestBoundedSlotPreservesRenderedPaths(t *testing.T) {
	root := &region{kind: regionExternal}
	placeholder := &region{kind: regionPlaceholder, source: slot{region: root, path: "field:0/field:1"}}
	projection := &heapProjection{roots: map[*region]HeapRoot{root: {Kind: HeapParameter, Index: 2}}}
	for _, test := range []struct {
		name   string
		target slot
		path   string
		ok     bool
	}{
		{"root", slot{region: root}, "", true},
		{"limit", slot{region: root, path: "field:0/field:1/field:2"}, "field:0/field:1/field:2", true},
		{"beyond", slot{region: root, path: "field:0/field:1/field:2/field:3"}, "", false},
		{"placeholder limit", slot{region: placeholder, path: "field:2"}, "field:0/field:1/field:2", true},
		{"placeholder beyond", slot{region: placeholder, path: "field:2/field:3"}, "", false},
		{"empty steps at limit", slot{region: root, path: "//"}, "//", true},
		{"empty steps beyond", slot{region: root, path: "///"}, "", false},
		{"unnamed", slot{region: &region{kind: regionSite}}, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := projection.boundedSlot(test.target)
			if ok != test.ok || got.Path != test.path {
				t.Fatalf("bounded slot = %v, %t; want path %q, %t", got, ok, test.path, test.ok)
			}
			if !ok && got != (HeapSlot{}) {
				t.Fatal("rejected slot published partial evidence")
			}
			if ok && got.Root != projection.roots[root] {
				t.Fatal("accepted slot changed its named root")
			}
		})
	}
}

func BenchmarkOrderedSlots(b *testing.B) {
	for _, size := range []int{0, 1, 8, 32} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			entries := make(map[slot]bool, size)
			for index := range size {
				entries[slot{region: &region{serial: size - index}}] = false
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if got := orderedSlots(entries); len(got) != size {
					b.Fatal("sorting lost a slot")
				}
			}
		})
	}
}

func BenchmarkBoundedSlot(b *testing.B) {
	for _, depth := range []int{0, SummaryPaths, SummaryPaths + 1} {
		b.Run(strconv.Itoa(depth), func(b *testing.B) {
			root := &region{kind: regionExternal}
			projection := &heapProjection{roots: map[*region]HeapRoot{root: {Kind: HeapParameter}}}
			path := strings.TrimSuffix(strings.Repeat("field:0/", depth), "/")
			target := slot{region: root, path: path}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, ok := projection.boundedSlot(target); ok != (depth <= SummaryPaths) {
					b.Fatal("projection changed its depth boundary")
				}
			}
		})
	}
}

// An opaque read of an owner slot can still select a field of its previous
// occupant. The relationship is possible identity, never a cleanup guarantee.
func TestPlaceholderProjectionAliases(t *testing.T) {
	for _, test := range []struct {
		name, left, right string
		aliases           bool
	}{
		{"field", "&value.mu", "&value.mu", true},
		{"sibling", "&value.mu", "&value.other", false},
		{"nested", "&value.child.mu", "&value.child.mu", true},
		{"nestedSibling", "&value.child.mu", "&value.child.other", false},
		{"element", "&value.items[0]", "&value.items[0]", true},
		{"otherElement", "&value.items[0]", "&value.items[1]", false},
		{"ownerIsNotField", "value", "&value.mu", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "placeholderprobe", `package placeholderprobe
import "sync"
type child struct { mu, other sync.Mutex }
type owner struct { mu, other sync.Mutex; child child; items [2]sync.Mutex }
func opaque(**owner)
func observe(any, any) {}
func body(value *owner) {
 before := `+test.left+`
 opaque(&value)
 observe(before, `+test.right+`)
}
`)
			calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("body"))
			args := calls[len(calls)-1].Common().Args
			proof := ProveMayAlias(args[0], args[1])
			if proof.Aliases != test.aliases {
				t.Errorf("possible placeholder projection = %+v, want aliases %t", proof, test.aliases)
			}
			if DefinitelySameValue(args[0], args[1]) {
				t.Error("opaque owner projection established exact identity")
			}
		})
	}
}

func TestStoredPathAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "pathallowance", `package pathallowance
type resource struct { n int }
type holder struct { value, other *resource }
type nested struct { inner holder }
type deep struct { inner nested }
func observe(any) {}
func exact(p, other *resource) { h := &holder{p,other}; observe(h) }
func replaced(p, other *resource) { h := &holder{p,other}; h.value = other; observe(h) }
func two(p, other *resource) { h := &nested{holder{p,other}}; observe(h) }
func three(p, other *resource) { h := &deep{nested{holder{p,other}}}; observe(h) }
`)
	for _, test := range []struct {
		name string
		path []string
	}{
		{"exact", []string{"field:0"}},
		{"replaced", nil},
		{"two", []string{"field:0", "field:0"}},
		{"three", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := heapObservation(t, fn)
			root := call.Common().Args[0]
			// observe's interface box is transparent to the graph but not to
			// the structural selection walk. Use its concrete source for both.
			root, _ = ssaflow.UnwrapTransparentValue(root, ssaflow.TransparentMakeInterface)
			baseline := ProveStoredPathWithin(root, fn.Params[0], call, nil)
			if baseline.Proven() != (test.path != nil) || !slices.Equal(baseline.Path, test.path) {
				t.Fatalf("default stored path = %+v, want %v", baseline, test.path)
			}
			for limit := 0; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := ProveStoredPathWithin(root, fn.Params[0], call, budget)
				if limit == 0 || budget.Exhausted() || budget.PoolExhausted() {
					if got.Proven() || got.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("interrupted stored path = %+v", got)
					}
					continue
				}
				if got.State != baseline.State || got.Reason != baseline.Reason || !slices.Equal(got.Path, baseline.Path) {
					t.Fatalf("complete stored path = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("stored path never completed")
		})
	}
}

func TestStoredPathStructuralChildCap(t *testing.T) {
	source := `package pathchild
type holder struct { value *int }
func noop() {}
func observe(*holder) {}
func caller(p *int) { h := &holder{p};
` + strings.Repeat("noop()\n", proofs.QueryBudget+100) + "observe(h) }"
	fn := ssaflowtest.BuildPackage(t, "pathchild", source).Func("caller")
	call := heapObservation(t, fn)
	root := call.Common().Args[0]
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(proofs.QueryBudget)
	query := storedPathQuery{budget: pool, storageBudget: child, storage: NewStorage(child), target: fn.Params[0], observation: call}
	proof := storedPathProof(query.walk(root, nil, 2), pool, child)
	if proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() || pool.PoolExhausted() {
		t.Fatalf("structural storage child cutoff = %+v, parent exhausted %v", proof, pool.Exhausted())
	}
	fresh := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	query = storedPathQuery{budget: fresh, storageBudget: fresh, storage: NewStorage(fresh), target: fn.Params[0], observation: call}
	if path := query.walk(root, nil, 2); !slices.Equal(path, []string{"field:0"}) || fresh.Exhausted() {
		t.Fatalf("fresh structural path = %v, exhausted %v", path, fresh.Exhausted())
	}
}
