---
title: Understanding SSA
description: What static single assignment form is, why the analyzers reason over it instead of source, and how to read a dump.
sidebar:
  order: 2
---

Every gohawk analyzer that reasons about ownership, lifecycle, or control flow
works on the SSA form of a function, not on its syntax tree. This page
explains what that form is, why it makes the analyses possible, and how to
read the dump `gohawk ssa` prints. The [debugging reference](../debugging-reference/)
covers the dump commands themselves.

## What SSA is

Static single assignment form rewrites a function so that every value is
defined exactly once. There are no variables that change over time: each
assignment in the source becomes a fresh, uniquely named value, and every use
refers to the one definition that produced it.

Three ideas follow from that rule, and the rest of the form is built from
them.

- **Values and instructions.** A function becomes a list of instructions.
  Most instructions produce a value, written `tN`, that later instructions
  consume by name. A value's identity is the instruction that produced it,
  so "the file returned by `os.Open`" is one specific value that every later
  use names directly.
- **Basic blocks.** Instructions are grouped into blocks, each a straight
  line of code with one entry and one exit. `if`, loops, `&&`, `||`, and
  `select` become edges between blocks; the last instruction of a block is a
  jump, a conditional branch, or a return.
- **Merges.** When two paths assign different values to what was one source
  variable, the block where they meet begins with a `phi` instruction that
  selects the value according to which predecessor ran. A loop variable is a
  phi of its initial value and its next value. A variable that is never
  reassigned across a branch needs no phi at all.

Source variables whose address is taken, which a closure captures, or which a
deferred call must observe are the exception: they become a `local` or `new`
allocation, a cell, with explicit stores into it and loads out of it. The
cell is a value; what it holds at any point is whatever the reaching store
put there.

## Why it suits analysis

The analyzers make claims such as "this file is closed on every path that
returns" or "this goroutine's completion channel is received from before the
function exits". Each of those is a question about a specific value along
every path, and SSA is the form in which such questions have exact answers.

- **Identity is structural.** Because each value has one definition, asking
  whether two expressions denote the same file is a question about
  instructions, not names. `SameValue` and the transparent-form helpers
  answer it by following conversions, phis, and local load/store pairs, and
  nothing else, so an alias the analysis cannot see through is simply a
  different value rather than a guess.
- **Every path is explicit.** Blocks and edges are the control-flow graph, so
  "on every return path" is a walk over successors from the acquisition to
  each `return`. Feasibility comes from the same graph: an `if` on `err !=
  nil` has one successor where the error is nil and one where it is not, and
  the flow follows only the branch the evidence admits.
- **Merges are honest.** A phi says outright that a value has several
  origins. A proof that needs every origin to satisfy a property, such as a
  context that must be detached on every edge, checks each edge; a proof that
  needs only one uses the fold's any-edge form. Nothing has to reason about
  which assignment "won".
- **Cells separate the holder from the held.** A loaded value is what the
  cell contained at that load, not the cell. That distinction is why
  ownership proofs never peel loads: closing the file loaded from a cell says
  nothing about a different file stored there later. Identity resolution,
  which asks a weaker question, may look through the load.
- **Calls carry their shape.** A call instruction records whether its callee
  is a known function, a function value, or an interface method invocation,
  and a `defer` or `go` wraps the same call description. That is what lets a
  classifier label a consumption as settled, opaque, or irrelevant from the
  instruction alone.

The cost is that the form is not source. `defer file.Close()` is not a
statement to find; it is a `defer` instruction whose callee is a method value
on a specific SSA value. The analyzers accept that cost because it removes
ambiguity, and this page exists so a reader can pay it too.

## Reading a dump

The block below is the real output of `gohawk ssa -func CopyHeader` on this
small function, regenerated with the documentation so it cannot drift from
what the tool prints:

```go
func CopyHeader(path string, fallback string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	buffer := make([]byte, 64)
	count, err := file.Read(buffer)
	header := fallback
	if err == nil && count > 0 {
		header = string(buffer[:count])
	}
	log := func() { _ = header }
	log()
	return header, nil
}
```

<!-- gohawk:generated-ssa-example:start -->
```text
// tools/gendocs/ssaexample/example.go:13:6
# Name: github.com/kojah/gohawk/tools/gendocs/ssaexample.CopyHeader
# Package: github.com/kojah/gohawk/tools/gendocs/ssaexample
# Location: tools/gendocs/ssaexample/example.go:13:6
# Recover: 3
# Locals:
#   0:	t0 string
#   1:	t1 error
func CopyHeader(path string, fallback string) (string, error):
0:                                                                entry P:0 S:2
	t0 = local string ()                                            *string
	t1 = local error ()                                              *error
	t2 = os.Open(path)                                    (*os.File, error)
	t3 = extract t2 #0                                             *os.File
	t4 = extract t2 #1                                                error
	t5 = t4 != nil:error                                               bool
	if t5 goto 1 else 2
1:                                                       if.then P:1 S:0 idom:0
	*t0 = "":string
	*t1 = t4
	rundefers
	t6 = *t0                                                         string
	t7 = *t1                                                          error
	return t6, t7
2:                                                       if.done P:1 S:2 idom:0
	defer (*os.File).Close(t3)
	t8 = new [64]byte (makeslice)                                 *[64]byte
	t9 = slice t8[:64:int]                                           []byte
	t10 = (*os.File).Read(t3, t9)                        (n int, err error)
	t11 = extract t10 #0                                                int
	t12 = extract t10 #1                                              error
	t13 = new string (header)                                       *string
	*t13 = fallback
	t14 = t12 == nil:error                                             bool
	if t14 goto 6 else 5
3:                                                              recover P:0 S:0
	t15 = *t0                                                        string
	t16 = *t1                                                         error
	return t15, t16
4:                                                       if.then P:1 S:1 idom:6
	t17 = slice t9[:t11]                                             []byte
	t18 = convert string <- []byte (t17)                             string
	*t13 = t18
	jump 5
5:                                                       if.done P:3 S:0 idom:2
	t19 = make closure CopyHeader$1 [t13]                            func()
	t20 = t19()                                                          ()
	t21 = *t13                                                       string
	*t0 = t21
	*t1 = nil:error
	rundefers
	t22 = *t0                                                        string
	t23 = *t1                                                         error
	return t22, t23
6:                                                     cond.true P:1 S:2 idom:2
	t24 = t11 > 0:int                                                  bool
	if t24 goto 4 else 5


// tools/gendocs/ssaexample/example.go:25:9
# Name: github.com/kojah/gohawk/tools/gendocs/ssaexample.CopyHeader$1
# Package: github.com/kojah/gohawk/tools/gendocs/ssaexample
# Location: tools/gendocs/ssaexample/example.go:25:9
# Parent: CopyHeader
# Free variables:
#   0:	header *string
func CopyHeader$1():
0:                                                                entry P:0 S:0
	t0 = *header                                                     string
	return
```
<!-- gohawk:generated-ssa-example:end -->

Read it top to bottom with the following in mind.

- **The header.** `# Recover: 3` names the block that runs if a deferred call
  recovers from a panic, and `# Locals` lists the cells the function
  allocates. The named results became cells `t0` and `t1` because a deferred
  call could observe them; a function with no `defer` returns its results
  directly.
- **Block labels.** Each block shows its index, a name such as `if.then` or
  `cond.true` that records the source construct it came from, its
  predecessor and successor counts, and its immediate dominator (`idom`).
  Dominance is what `InstructionDominates` reads: block 2 dominates
  everything after the error check succeeded.
- **The call and its results.** `os.Open` returns a tuple; `extract t2 #0`
  and `extract t2 #1` split it into the file and the error. The file is `t3`
  from here on, and every later mention of it, including the deferred close,
  names `t3` directly.
- **The branch.** `t5 = t4 != nil:error` followed by `if t5 goto 1 else 2`
  is the error check. `SuccessBranch` recognizes this shape, which is how the
  flow knows the file is owned only on the path through block 2.
- **The defer.** `defer (*os.File).Close(t3)` records the callee and its
  receiver at registration time; `rundefers` marks the points where deferred
  calls execute, immediately before each `return`. The completion search
  asks whether such a defer covers every return.
- **The captured variable.** `header` became a cell, `t13 = new string
  (header)`, because the closure captures it. Both assignments are stores
  into `t13`, the closure is created with `make closure CopyHeader$1 [t13]`,
  and inside `CopyHeader$1` the free variable `header` is loaded with
  `*header`. A variable that no closure captured would instead have
  produced a `phi` at block 5 merging `fallback` and `t18`.
- **The condition.** `err == nil && count > 0` became two blocks: block 2
  branches on `t14`, and block 6 (`cond.true`) evaluates the second operand
  only when the first held. Short-circuit evaluation is control flow, not an
  expression.
- **The conversion.** `convert string <- []byte (t17)` is a `Convert`
  instruction that changes representation. The value-provenance folds treat
  conversions as transparent only when a caller opts in, because a converted
  value may no longer carry the same obligation.

## The instruction vocabulary

The forms the analyzers handle, grouped by what they express. Every SSA type
that appears in an analyzer, in `ssaflow`, or in a pass is listed here;
`TestDocumentationReferencesResolve` fails when a new one is not.

| Form | What it expresses |
| --- | --- |
| `Function`, `Package`, `BasicBlock` | the unit being analyzed, its package, and one straight-line block |
| `Value`, `Parameter`, `FreeVar`, `Global`, `Const`, `Builtin` | the things instructions consume: a parameter, a captured variable, a package-level variable, a constant, or a builtin such as `close` or `append` |
| `Alloc` | a cell for an addressed or captured variable; `local` for stack cells, `new` for heap cells and composite literals |
| `Store`, `UnOp` | a store into a cell, and a load out of one (`*cell`) among the other unary operators |
| `Phi` | a merge of the values arriving from each predecessor block |
| `If`, `Return`, `Panic` | the instructions that end a block: a conditional branch, a normal return, and a panic |
| `Call`, `CallCommon` | a call and the description shared by every call-like instruction: the callee, its arguments, and whether it is an interface invocation |
| `Defer`, `Go` | a deferred call and a launched goroutine, each wrapping a `CallCommon` |
| `MakeClosure` | a closure value created from a function and the bindings for its free variables |
| `Extract` | one result of a multi-result call, a `select`, or a map iteration step |
| `FieldAddr`, `Field` | the address of a struct field, and a field read from a struct value |
| `IndexAddr`, `Index`, `Lookup`, `Slice` | element addresses, element reads, map reads, and slicing |
| `Range`, `Next` | iteration over a map or string; slices lower to index loops instead |
| `MakeChan`, `Send`, `Select`, `SelectState`, `MapUpdate` | channel creation and sends, a `select` with its cases, and a map write |
| `MakeInterface`, `ChangeInterface`, `TypeAssert` | placing a value in an interface, converting between interfaces, and narrowing one |
| `Convert`, `ChangeType` | a representation change, and a change of named type over the same representation |
| `BinOp` | an arithmetic or comparison operator, including the `!= nil` checks the flow reads |
| `DebugRef` | a source-position annotation with no runtime effect, skipped by every analysis |

Two facts about the vocabulary matter more than any single row. A `Call`
with `IsInvoke` set has no static callee, so nothing downstream can be
proven about it; and a `MakeClosure` is a value, not a call, so creating a
closure never counts as running it.
