# SSA in gohawk

The public [Understanding SSA](../understanding-ssa.md) page explains the form
for readers. This note keeps the contributor detail: which helpers answer
which question, how the analyzers read each part of a dump, and the full
vocabulary of SSA forms the analyzers handle. The architecture tests require
every `*ssa` type named in analyzer, `ssaflow`, `lifecycle`, `heapmodel`,
`resourcemodel`, or pass code to appear in the vocabulary table below.

## The program

An `ssa.Program` owns packages, method sets, and compiler-generated method
wrappers. Exact interface dispatch uses this existing program to obtain the
method for a proven concrete receiver; it does not fabricate SSA instructions
or enumerate every implementation in the program.

## Why the analyzers use SSA

The analyzers make claims such as "this file is closed on every path that
returns" or "this goroutine's completion channel is received from before the
function exits". Each of those is a question about a specific value along
every path, and SSA is the form in which such questions have exact answers.

- **Identity is structural.** Because each value has one definition, asking
  whether two expressions mean the same file is a question about instructions,
  not names. `MayAlias` and the transparent-form helpers answer it by
  following conversions, phis, and local load/store pairs, and nothing else,
  so an alias the analysis cannot see through is just a different value rather
  than a guess.
- **Every path is explicit.** Blocks and edges are the control-flow graph, so
  "on every return path" is just a walk over successors from where the value
  is acquired to each `return`. Whether a branch can actually run comes from
  the same graph: an `if` on `err != nil` has one successor where the error is
  nil and one where it is not, and the flow follows only the branch the
  evidence allows.
- **Merges are honest.** A phi says plainly that a value came from more than
  one place. A proof that needs every origin to hold some property — a context
  that must be detached on every edge, say — checks each edge; a proof that
  needs only one uses the any-edge form of the walk. Nothing has to work out
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

## Reading a dump as an analyzer does

The walkthrough below refers to the generated `CopyHeader` dump on the public
page.

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
| `Jump` | an unconditional edge to the next block; it performs no work, so a terminal completion signal can precede a jump to a shared return |
| `Call`, `CallCommon` | a call and the description shared by every call-like instruction: the callee, its arguments, and whether it is an interface invocation |
| `Defer`, `Go` | a deferred call and a launched goroutine, each wrapping a `CallCommon` |
| `RunDefers` | executes registered deferred calls in reverse order before returning; registration captures arguments, but does not execute the deferred effect |
| `MakeClosure` | a closure value created from a function and the bindings for its free variables |
| `Extract` | one result of a multi-result call, a `select`, or a map iteration step |
| `FieldAddr`, `Field` | the address of a struct field, and a field read from a struct value |
| `IndexAddr`, `Index`, `Lookup`, `Slice` | element addresses, element reads, map reads, and slicing |
| `Range`, `Next` | iteration over a map or string; slices lower to index loops instead |
| `MakeChan`, `Send`, `Select`, `SelectState`, `MapUpdate` | channel creation and sends, a `select` with its cases, and a map write |
| `MakeMap`, `MakeSlice` | newly allocated maps and slices; nilness of the result does not establish resource ownership |
| `MakeInterface`, `ChangeInterface`, `TypeAssert` | placing a value in an interface, converting between interfaces, and narrowing one |
| `Convert`, `ChangeType` | a representation change, and a change of named type over the same representation |
| `MultiConvert` | a conversion between type parameters whose instantiations may need different representation changes |
| `SliceToArrayPointer` | a slice converted to a pointer to an array over the same backing store |
| `BinOp` | an arithmetic or comparison operator, including the `!= nil` checks the flow reads |
| `DebugRef` | a source-position annotation with no runtime effect, skipped by every analysis |

Two facts about the vocabulary matter more than any single row. A `Call`
with `IsInvoke` set has no static callee, so nothing downstream can be
proven about it; and a `MakeClosure` is a value, not a call, so creating a
closure never counts as running it.
