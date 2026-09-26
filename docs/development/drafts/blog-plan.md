# Lock analysis in Go

"To do two things at once is to do neither."
— Publilius Syrus
"I feel thin, sort of stretched, like butter scraped over too much bread."
— J.R.R. Tolkien, The Fellowship of the Ring

A couple of months ago I was working on a medium-sized Go project built around Git, and I got ambitious: I decided to parallelize as much of it as I could to make it faster.

This introduced a whole bird's nest of bugs (heh). There's a particular kind of tired you feel at 1am staring at a race-detector report, where every fix just seems to relocate the bug instead of removing it. You start to distrust your own eyes. That was me for about two weeks, reading the same functions over and over, certain the bug had to be somewhere I simply wasn't looking.

I did what everyone does now and asked a coding assistant for help. It was confident, fast, and wrong in all the familiar ways. It "fixed" races by adding locks in all the same places I'd thought of, and reintroduced the same deadlocks I'd just spent a night removing. There's something uniquely demoralizing about an assistant handing your own bug back to you with better comments. It was like pair-programming with someone who'd read every Go tutorial ever written and internalized none of the fear.

## A peek into the abyss

Here's the shape of bug I kept seeing. Each function looks correctly locked on its own:

```go
type Registry struct {
    mu      sync.Mutex
    workers map[string]*Worker
}

func (r *Registry) StopAll() {
    r.mu.Lock()
    defer r.mu.Unlock()
    for id := range r.workers {
        r.stop(id)
    }
}

func (r *Registry) stop(id string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.workers[id].Stop()
    delete(r.workers, id)
}
```

The deadlock lives in the space between the two functions, in the call edge nobody looked across. Go mutexes aren't reentrant, so the first loop iteration hangs forever, holding a lock it will never release. An assistant reviewing either method alone would approve it. The bug only exists in the composition. I stared at those two functions for an embarrassingly long time before the penny dropped, and it's why I stopped trusting my eyes and started wanting a tool that didn't have any.

This is the part where Go started to feel less like the source of my problems and more like the way out. Being statically typed, it has the benefit of _static analysis_ tools over a dynamically-typed language like Python, and I don't just mean that as a technical observation. It means the language keeps enough of a record of what you meant that a tool can double-check you. Fast to compile, close to the metal, and willing to be inspected: that's a rare combination, and it's why Go is one of my go-to languages :smirk:

In spite of all this talk lately about AGI and RSI, I don't really trust coding assistants to be able to write Go code correctly :cry-laugh: AI models have been trained en masse on Go code from the internet, but people don't always follow the best coding conventions, and that really gets reflected in the Go code. (I've written a `go-coding-conventions` skill to encourage AI to write neater and extensible Go code, but that's only a soft guard; sometimes you need hard checks.)

There's an irony I keep coming back to: these models learned Go from the internet's Go code, which means they learned all of our bad habits too. Every shortcut some tired programmer took at midnight is in there somewhere, laundered into "idiomatic style."

Back to the problem at hand. By this point, I had already enabled a bunch of static analysis tools in my project, including Staticcheck (the OG), NilAway, go-critic, etc etc. But none of them seemed to be able to detect this class of error.

So I decided to try and roll my own analyzer.

I'll be honest about what that decision felt like: stubbornness, curiosity, and the classic "how hard could it be" (the four most expensive words in software). Nothing else could see the bug. Fine. I'd teach something to see it.

"When lemons give you life, lemonade make." -- _Two threads trying to describe the problem we're about to tackle_

## Go's analysis tooling

Once I'd decided to build the thing, I started taking stock of what Go gives you for free, and the list kept getting longer. It's one of the best languages in existence for static analysis, and I don't think that's an accident. Go is conservative about features (ten years to get generics!), it refuses macros and operator overloading and the rest of the complexity candy, and what you're left with is a language small enough to reason about. C# is the only real rival I can name, and that's because Microsoft poured years into the Visual Studio experience. Rust, for all its virtues, won't even give you a stable compiler API. New language features come first, tooling second.

Go went the other way. The standard library hands you the full production compiler pipeline (`go/ast`, `go/types`, the works), and the `go/analysis` module gives you a genuinely pleasant framework for writing your own diagnostics.

On top of all this, there is some icing on the cake that only Go offers... I'll admit I fell a little in love with this part. It's rare for a language to hand you the compiler's own eyes and say: go look for yourself.

## SSA is served

Suppose we acquire a mutex and check whether we're ready to continue. To keep the dump short, I've left out the actual work. The bug is the early return:

```go
func Update(mu *sync.Mutex, ready bool) {
    mu.Lock()
    if !ready {
        return
    }
    mu.Unlock()
}
```

Both `Lock` and `Unlock` are present, so searching for matching calls won't get us very far. We need to follow the paths through the function.

Go's SSA package (`go/ssa`) gives us a representation we can use for that. SSA stands for *static single assignment*, meaning each value in the representation is defined once. It also organizes instructions into basic blocks, with branches connecting them. Here's what the SSA of this example looks like:

```text
func Update(mu *sync.Mutex, ready bool):
0:                                                                entry P:0 S:2

	t0 = (*sync.Mutex).Lock(mu)                                          ()
	if ready goto 2 else 1
1:                                                       if.then P:1 S:0 idom:0
	return
2:                                                       if.done P:1 S:0 idom:0
	t1 = (*sync.Mutex).Unlock(mu)                                        ()
	return
```
SSA is designed for analysis programs that trace dataflow. Here, this representation tells us that:

- Block `0` acquires the lock and branches.
- If `ready` is false, execution goes to block `1`, which returns immediately.
- Only block `2` unlocks the mutex.
- The `t0` and `t1` names are SSA temporaries.

We can see the problem clearly: both method calls use the same `mu` parameter. That's more useful than matching variable names in the source, since we're following the _value_ the calls actually receive.

## Facts: one step further

What if the caller delegates the unlock to a helper?

```go
func Refresh(mu *sync.Mutex) {
	mu.Lock()
	// ... do the work ...
	finish(mu)
}

func finish(mu *sync.Mutex) {
	mu.Unlock() // this is hidden inside a helper!
}
```

Now the absence of an `Unlock` in the caller doesn't tell us whether anything is wrong. We have to inspect the helper. And we can't just search that helper for an unlock, because it might have its own early return. Or it might unlock something else entirely.

Moving code into another function shouldn't break an analyzer's understanding of it. “Please undo your refactoring so my check can pass” would be a pretty annoying feature.

This is where SSA stopped being enough on its own. It showed me the calls, arguments, and branches, but I still had to derive their meaning. Does a helper finish the cleanup? Does it do so on every normal return? Which argument does that guarantee apply to?

I ended up building a separate analysis pass that records summaries of function behavior. A caller can use a summary instead of rediscovering everything about the function it calls. For lifecycle checks, for example, a summary can establish that a helper closes a particular argument before returning normally.

To see one of those summaries, let's briefly switch from locks to a file. This is the lifecycle fact system's output, not a dump of lock-order relationships:

```go
func CloseFile(file *os.File) error {
    return file.Close()
}
```

The same example package contains this helper. Run:

```sh
gohawk dump facts -func CloseFile ./...
```

Below the function and source-location header, the dump contains this parameter row:

```text
  0 file: Closed
```

`0` is the parameter index, `file` is its name, and `Closed` records the cleanup action. The helper calls `Close` before returning normally. It still returns any error from `Close` to its caller; the summary isn't a promise that the operation succeeded.

That gives a caller useful information it wouldn't get from an SSA call instruction alone. It can recognize cleanup delegated to this helper instead of treating the call as opaque.

The qualification matters. A function that *sometimes* closes its argument doesn't give the caller the same guarantee as one that always does. And a function we haven't analyzed isn't equivalent to one we've analyzed and found to do nothing.

## Taking a dive: heap modelling

SSA is honest about values it can see: locals, parameters, things sitting at known addresses. But real programs don't leave their valuables on the kitchen counter. They tuck them into struct fields, slices, maps. The moment a value is stored into mutable storage, SSA's def-use chains go quiet about what a later load will find there.

Heap modelling is the layer that picks up that question. It reasons about abstract locations, things like "field `out` of this struct" or "index 1 of that slice," and tracks which object each one currently holds. The inferences still come from the SSA graph. They just go one step further than SSA alone, because we're no longer asking about values at known addresses. We're asking about identity after a trip through the heap.

Here's where it matters. Suppose a resource is stored in a struct in one function and cleaned up through a copy of that struct in another:

```go
type job struct {
    out *os.File
}

func run(path string) error {
    // This file is always closed. But to prove it, we need to follow f...
    f, err := os.Open(path)
    if err != nil {
        // We note that f isn't closed on this path, but err being non-nil means we don't have that obligation
        return err
    }
    // ...through this storage in a field...
    j := job{out: f}
    // through an interprocedural call...
    return finish(j)
}

func finish(j job) error {
    // ...see that j is a copy of the original j struct...
    // ...see that j.out here is the same as the original file f...
    // ...and see that it gets closed on all return paths.
    return j.out.Close()
}
```

For `finish` to earn the kind of summary the last section showed, the analysis has to follow `f` into the struct field, across the call boundary, and back out through the load in `finish`, and prove it's the same object at every step. Without that, the whole summary layer goes blind the moment anyone uses a struct.

Fields aren't the only place a value can hide. The same question comes up for arrays, and the abstract location changes from "field `out`" to "index 0":

```go
type batch struct {
    files [2]*os.File
}

func runBatch(path string) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    // f goes into index 0 of an array, inside a field, of a struct passed by value.
    return finishBatch(batch{files: [2]*os.File{f, nil}})
}

func finishBatch(b batch) error {
    // The summary for finishBatch says: "closes index 0 of field files of its parameter."
    // The caller stored f at exactly that path, so f is closed.
    return b.files[0].Close()
}
```

The summary is indexed by path, not by "somewhere in the argument." That precision cuts both ways. If `finishBatch` closed `b.files[1]` instead, the caller would still be holding an open file at index 0, and gohawk reports it. A helper that cleans up the other element is not cleaning up yours. Before the model tracked paths, any resource anywhere in the argument got credited, which is exactly the kind of rumor the next paragraph warns about.

The model is deliberately pessimistic. If the struct escapes into a function we can't see, or the field is written on only one branch of an `if`, or the index is computed at runtime, the answer isn't a guess. It's "I don't know." Unknown evidence is never a positive result. An analyzer that can't prove identity must not claim it. That's the difference between a fact and a rumor.

## A pass of the history books

While building gohawk, I learned a lot of about the existing landscape of compiler toolchains and static analyzers.

The biggest influence on gohawk is Meta's [Infer](https://fbinfer.com/docs/separation-logic-and-bi-abduction/), specifically its trick of compositional summaries. Analyze each function once, write down what it needs and what it changes, and let callers use the summary instead of re-deriving everything. Underneath, Infer has serious separation-logic machinery for reasoning about just the slice of the heap a function touches. while Gohawk's capabilities don't come anywhere near to the level of Infer's, we certainly do borrow many techniques from them.

The [Clang Static Analyzer](https://clang.llvm.org/docs/ClangStaticAnalyzer.html) was also an interesting reference point while I was building gohawk. Even though it comes from the C/C++ world, which is pretty orthogonal to Go's memory-safe model -- I still drew a lot of inspiration from it for things like the fact system, bounding the search budget for potentially expensive searches, and resource tracking. Plus, it has a pretty interesting origin story of its own.

For locking specifically, I kept coming back to Linux's [lockdep](https://cdn.kernel.org/doc/html/latest/locking/lockdep-design.html). Lockdep watches a running kernel and records which lock classes get acquired while holding which others; a conflicting ordering somewhere else flags a potential deadlock without anyone having to actually deadlock first. The elegant move is the level of abstraction: the program locks real objects, but the *validator* groups them into classes and reasons about roles. gohawk does something similar to this -- outside of a function, it becomes nearly impossible to trace the actual identity of the objects that are being locked on, so instead we simply group locks by their class instead when enforcing a certain ordering.

And then there's [GCatch](https://github.com/system-pclub/GCatch), a research tool that hunts concurrency bugs in Go, including the ones where goroutines wedge themselves on channels instead of locks. I'm considering adapting its richer concurrency model for future use, but more on that later.

## Real-world bugs we've fixed

In Caddy, gohawk found a lock-order inversion in `UsagePool`. When a constructor failed, the error path held an entry lock while acquiring the pool lock. `Range` took those locks in the opposite order. Put the two operations together and they could deadlock—the same kind of problem that got me interested in writing an analyzer in the first place.

The [merged fix](https://github.com/caddyserver/caddy/pull/7968) releases the entry lock before acquiring the pool lock. This affected callers combining `LoadOrNew` with `Range`, which external modules can do, rather than Caddy's built-in pool usage.

In Docker, the process-ownership check found error paths that returned without waiting for a child process. Investigating that code also uncovered a pipe-handling deadlock: Docker could wait for stdout to finish while the child was blocked writing to stderr.

The [fix](https://github.com/moby/moby/pull/53517) handed input and output handling back to `os/exec` and used `Run`. That let the standard library drain both output streams and wait for the child, including when copying failed. The patch fixed both problems and was merged upstream. gohawk led us to the missing wait; the pipe deadlock came out of investigating the finding.

<!-- Editorial follow-up: add the Kubernetes example once its PR link and merge status are verified. -->

## Conclusion

I expected SSA to get me further than it did. Building the fact system was the first surprise. Still finding false positives after scanning more than 500 repositories was the next.

Sometimes another exception helps. With `globalstate`, enough of them piled up that I started considering whether to disable the check entirely. That's a less exciting part of writing an analyzer, but it matters if people are going to trust it.

Next time: resource lifetimes, and what happens when cleanup becomes somebody else's job.
