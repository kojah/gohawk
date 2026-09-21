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

*Show Go code alongside its SSA representation. Explain what SSA reveals and what it doesn’t tell you about ownership and cleanup.*

## Facts, facts, facts

_Explain that SSA itself is insufficient. Talk about the fact system that has to be built on top of SSA via a separate pass, and explain why it's necessary._

## A walk through history

*Introduce the Clang Static Analyzer and how it reasons about program state via its fact system. Also talk about lockdep from the linux kernel -- rather than attempting to lock specific objects, we just try and lock different classes.*

## Going back to Go...

Give examples of how we build up our fact system to perform lock analysis.

## Conclusion

*Reflect on the gap between having SSA available and building useful analysis on top of it.*

_State that next post will be about tracking resource lifetimes._