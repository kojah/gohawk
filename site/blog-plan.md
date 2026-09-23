# Lock analysis in Go

"To do two things at once is to do neither."
— Publilius Syrus
"I feel thin, sort of stretched, like butter scraped over too much bread."
— J.R.R. Tolkien, The Fellowship of the Ring

A couple of months ago, I was working on a medium-scale Go project that relied on Git. In order to speed things up, I made efforts to parallelize the code as much as possible.

This introduced a whole bird's nest of bugs (heh). Even if I asked a coding assistant to help with the problem, I noticed it would keep making certain mistakes. Here's a real-world example of a code pattern where it'd usually trip up:

```
...
```

Go, being a statically typed language, has the benefit of _static analysis_ tools over a dynamically-typed language like Python. In my opinion, it makes Go one of the best choices available for rapid development: not only is it fast to compile and iterate, not only is it close to the metal when it runs, but it also has pretty sweet tooling. You could say it's why Go is one of my go-to langauges :smirk:

In spite of all this talk lately about AGI and RSI, I don't really trust coding assistants to be able to write Go code correctly :cry-laugh: AI models have been trained en masse on Go code from the internet, but people don't always follow the best coding conventions, and that really gets reflected in the Go code. (I've written a `go-coding-conventions` skill to encourage AI to write neater and extensible Go code, but that's only a soft guard; sometimes you need hard checks.)

Back to the problem at hand. By this point, I had already enabled a bunch of static analysis tools in my project, including Staticcheck (the OG), NilAway, go-critic, etc etc. But none of them seemed to be able to detect this class of error. So I decided to try and roll my own analyzer...

<another quote>

## Go's analysis tooling

Go, as a language, is uniquely positioned as one of the best languages to perform static analysis in. (I would say it's only rivaled in this department by C#, which has had extensive tooling developed around it by MS to support the Visual Studio experience.) In contrast to a language like Rust, which lacks a stable compiler API in order to accelerate the development of new language features, Go has tended to favor simplicity and is more conservative about adding new features (after all, it took us 10 years to get generics!). The language also deliberately omits features that would add a lot of complexity, such as macros or operator overloading.

Support for analyzers is also first-class. The standard library exposes the full production compiler pipeline (including `go/ast`, `go/types`, etc). And on top of that, the `go/analysis` module provides a modular and easy framework for authoring your own diagnostics.

On top of all this, there is some icing on the cake that only Go offers. Introducing...

## SSA is served

<!-- Editorial note: this is an illustrative example, not yet the real project example from the introduction. -->

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

Go's SSA package gives us a representation we can use for that. SSA stands for *static single assignment*, meaning each value in the representation is defined once. It also organizes instructions into basic blocks, with branches connecting them.

The example lives in [`site/examples/lock-analysis/example.go`](examples/lock-analysis/example.go). From that directory, we can ask gohawk to dump the function:

```sh
gohawk ssa -func Update ./...
```

Here's the actual output, with only the package and source-location header removed:

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

Block `0` acquires the lock and branches. If `ready` is false, execution goes to block `1`, which returns immediately. Only block `2` unlocks the mutex. The `t0` and `t1` names are SSA temporaries. We don't need the extra block metadata to see the problem.

Both method calls use the same `mu` parameter. That's more useful than matching variable names in the source: we're following the value the calls actually receive.

<!-- Dump generated with Go 1.27.0 and golang.org/x/tools v0.49.0. Formatting may change with toolchain updates. -->

That's useful infrastructure to get without writing a compiler. The [`go/ssa` package](https://pkg.go.dev/golang.org/x/tools/go/ssa) supplies the representation, and we can concentrate on the questions we want to ask about it.

Or so I thought. I'd underestimated how much work was hiding inside “ask questions.”

## Facts, facts, facts

What if the caller delegates the unlock to a helper?

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
gohawk facts -func CloseFile ./...
```

Below the function and source-location header, the dump contains this parameter row:

```text
  0 file: Closed
```

`0` is the parameter index, `file` is its name, and `Closed` records the cleanup action. The helper calls `Close` before returning normally. It still returns any error from `Close` to its caller; the summary isn't a promise that the operation succeeded.

That gives a caller useful information it wouldn't get from an SSA call instruction alone. It can recognize cleanup delegated to this helper instead of treating the call as opaque.

The qualification matters. A function that *sometimes* closes its argument doesn't give the caller the same guarantee as one that always does. And a function we haven't analyzed isn't equivalent to one we've analyzed and found to do nothing.

There's quite a lot of engineering in preserving those distinctions. I'd expected to spend most of my time writing checks on top of SSA. Building the layer those checks needed turned out to be a project of its own.

## A walk through history

There are older tools worth looking at here. People have been trying to persuade programs to clean up after themselves for a while.

Meta's [Infer](https://fbinfer.com/docs/separation-logic-and-bi-abduction/) has been a major influence on gohawk, especially its compositional summaries and heap modelling. The idea is to analyze a function on its own, summarize what it needs and what it changes, and use that summary when analyzing its callers. Its separation-logic foundations let it reason about the part of memory a function touches without having to describe the entire heap every time.

That's an appealing way to approach the helper problem from the previous section. A call can tell us something about what happened to the objects passed through it, even when their state lives behind pointers and fields. I'm borrowing ideas here, not claiming that gohawk implements Infer's analysis engine.

The [Clang Static Analyzer](https://clang.llvm.org/docs/ClangStaticAnalyzer.html) uses symbolic execution to explore paths through C and C++ programs. It tracks program state and constraints along the way, giving its checks context for judging later operations. A pointer's history matters, not just the expression currently using it.

That's the useful connection to gohawk: an operation becomes meaningful when we know what happened before it. Clang's program-state machinery and Go's analysis facts aren't interchangeable, though. They're different ways of supporting reasoning beyond an isolated statement.

For locking specifically, another interesting reference is Linux's lockdep.

[Lockdep](https://cdn.kernel.org/doc/html/latest/locking/lockdep-design.html) observes locking while the kernel runs and records dependencies between lock classes. If code acquires B while holding A, that establishes an ordering relationship. A conflicting relationship elsewhere can reveal a potential deadlock without requiring that deadlock to happen during the test.

The program still locks actual objects. The *validator* groups locks into classes so it can reason about their roles without treating every new instance as an unrelated problem.

That distinction is useful when analyzing source code, too. We may know which mutex field an operation accesses without knowing the runtime identity of every object containing it.

Closer to Go, there's [GCatch](https://github.com/system-pclub/GCatch), a research tool for finding concurrency bugs, including blocking bugs caused by channel misuse. Locks are only part of the story when goroutines can also get stuck waiting to send or receive.

GCatch is a direction I'd like to explore for gohawk's model in the future, to catch more of those interactions. That's a possible extension, not something gohawk already implements. It would also bring us back to the same question: can we understand enough of the interaction to report a useful bug without flagging working code?

## Going back to Go...

Suppose a store and its entries each have a mutex. One method takes the store lock and then an entry lock. Another method does the reverse.

Either method can look reasonable on its own. The problem appears when we put their orderings together.

For struct fields, gohawk's lock-order check compares the mutex declarations. It can identify that one field is acquired before another in one place and after it elsewhere. It doesn't need to claim that it knows every object those receivers could point to.

That also sets a limit on the conclusion. Conflicting orders are a hazard, not a prediction that these exact calls will deadlock on the next run. Cases that depend on the runtime ordering of two instances of the same mutex field need different evidence.

A finding in [Caddy](https://github.com/caddyserver/caddy/pull/7968) led to a merged fix for an error path that acquired two locks in the opposite order to another path.

<!-- Editorial follow-up: add a compact, verified before-and-after excerpt from the Caddy fix. The example package reproduces the SSA and fact excerpts above. Keep lifecycle summaries and lock-order evidence distinct; do not imply that the lifecycle fact format exports lock-order relationships. -->

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
