---
title: Understanding SSA
description: What static single assignment form is, why gohawk reads it instead of source, and how to read a dump.
sidebar:
  order: 2
---

gohawk's analyzers read the SSA form of your functions, not their source text.
This page explains what that form is, why it helps, and how to read the output
of `gohawk ssa`.

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

## Why gohawk uses it

gohawk asks questions like "is this file closed on every path that returns?"
SSA makes those questions precise.

- **A value is defined in one place.** To ask whether two expressions mean the
  same file, gohawk compares values, not variable names. A value it cannot
  trace back is treated as a different value, not guessed at.
- **Every path is visible.** Blocks and the edges between them are the
  control-flow graph, so "every return path" is a walk over that graph. An
  `if err != nil` splits into a path where the file was opened and one where it
  wasn't.
- **Merges are visible.** When a value can come from more than one place, a
  `phi` says so.
- **Calls show their kind.** A call records whether it goes to a known
  function, a function value, or an interface method. `defer` and `go` wrap
  the same call.

The cost is that SSA is not your source. `defer file.Close()` is not a
statement to find; it is a `defer` instruction on a specific value. This page
exists to make that translation easy.

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
  recovers from a panic. `# Locals` lists the cells the function allocates.
  The named results became cells `t0` and `t1` because a deferred call could
  observe them.
- **Block labels.** Each block shows its index, a name such as `if.then` that
  records the source construct it came from, and how many blocks lead into and
  out of it.
- **The call and its results.** `os.Open` returns two results. `extract t2 #0`
  and `extract t2 #1` split them into the file and the error. The file is `t3`
  from here on, and every later use names `t3`, including the deferred close.
- **The branch.** `t5 = t4 != nil:error` followed by `if t5 goto 1 else 2` is
  the error check. The file is only open on the path through block 2.
- **The defer.** `defer (*os.File).Close(t3)` records the call when the defer
  runs; `rundefers` marks where deferred calls execute, just before each
  `return`.
- **The captured variable.** `header` became a cell, `t13 = new string
  (header)`, because the closure captures it. Both assignments are stores into
  `t13`, and the closure `CopyHeader$1` reads it with `*header`.
- **The condition.** `err == nil && count > 0` became two blocks, because the
  second half only runs when the first is true.
- **The conversion.** `convert string <- []byte (t17)` turns the byte slice
  into a string.

## Common instructions

| Form | What it means |
| --- | --- |
| `Call` | a function or method call |
| `Extract` | one result of a call that returns several |
| `If`, `Jump`, `Return` | a conditional branch, an unconditional edge, and a return |
| `Phi` | a value that depends on which path reached this block |
| `Alloc`, `Store`, `UnOp` | a cell for a variable, a store into it, and a load out of it (`*cell`) |
| `Defer`, `RunDefers` | registering a deferred call, and running the registered ones before a return |
| `Go` | starting a goroutine |
| `MakeClosure` | creating a closure; it does not run it |
| `MakeChan`, `Send`, `Select` | making a channel, sending on it, and a `select` statement |
| `FieldAddr`, `IndexAddr` | the address of a struct field or of an element |

The full list of forms gohawk handles, and how the analyzers use each one, is in
the [SSA development notes](https://github.com/kojah/gohawk/blob/main/docs/development/ssa-in-gohawk.md).
