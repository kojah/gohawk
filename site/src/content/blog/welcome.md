---
title: Why I’m building gohawk
description: Following a missing cleanup through error paths, helper functions, and the point where an analyzer should admit it doesn't know.
date: 2026-09-11
draft: true
---

I've been building gohawk, a static analysis tool for Go, to catch resource-management and concurrency bugs. Go takes care of memory, but it won't wait for a child process or stop a goroutine just because we've lost interest in it. Fair enough. It doesn't know what we intended either.

One of gohawk's findings was in Docker. A function started a child process and waited for it at the end, but some error paths returned before reaching the wait. The [fix was merged](https://github.com/moby/moby/pull/53517).

That sounds like a small thing to check. Find a start, find a wait, complain if one's missing. Here's a simplified version of the code:

```go
func run(command *exec.Cmd) error {
    if err := command.Start(); err != nil {
        return err
    }
    if err := doSomething(); err != nil {
        return err // The child started, but we never wait.
    }
    return command.Wait()
}
```

Both calls are there. Our hypothetical search has done its job and is now confidently wrong.

The first error return is fine: the process didn't start. The second is different. We started something, then left without waiting for it. The analyzer needs to follow what can happen between the calls, including the paths we'd rather not have taken.

## Following the error path

This is where looking at the shape of the source stops being enough. We need to know that the command being waited on is the one we started, and whether we can return after starting it without reaching that wait.

Go provides an intermediate representation called SSA that helps with this. It gives an analyzer values and a graph of the paths through a function to work with. I'll get into SSA in a separate post. For now, the useful part is that we can ask about the path through the function, rather than whether a call appears somewhere in the text.

The Docker fix was nicer than adding a `Wait` before every error return. The code had been handling the child's input and output itself. Attaching those buffers to `exec.Cmd` and using `Run` let the standard library handle the I/O and wait for the child, including when copying failed. There were fewer paths for the code to get wrong.

So we can follow the paths and check for cleanup. What happens when someone extracts a helper?

## Someone tidied up the code

Suppose the wait moves into a helper called `finish`. The program still waits for the child, but our check no longer sees a direct call to `Wait`.

Reporting a bug here would be an odd reward for refactoring. Please put your code back in one enormous function, the linter gets confused.

We could look inside `finish`. But finding a `Wait` there isn't enough either. The helper might return early, or wait for a different command. We've arrived at the same question, one function further away.

gohawk uses function summaries to carry what it knows across calls. For this example, the useful summary is that a helper waits for a particular argument before it returns normally. A caller can use that information without treating every helper as a mystery. Building those summaries is enough of a topic for another post, too.

That handles a helper which finishes the work now. It doesn't yet explain a helper which takes the work away.

## Maybe it's somebody else's job now

Consider a resource passed to a longer-lived object. The current function returns without closing it because that object needs it, and will close it later. Adding a close at the return would introduce a bug rather than fix one.

The question has changed. We don't just need to find cleanup. We need to know whether the caller still owes it.

A method named `TakeOwnership` would be a convenient answer, but anyone can name a method that. The compiler doesn't check whether it kept its promise. gohawk needs evidence of the handoff and cleanup, not an encouraging name.

Sometimes it can establish that. Sometimes the resource disappears into code it can't follow. That's where I want the check to stop. Not knowing what happens next isn't evidence of a leak.

This means missing some bugs. I'm comfortable with that tradeoff. If an analyzer keeps asking you to explain perfectly ordinary code to it, eventually you'll stop inviting it to code review.

It also changes what a check needs in its tests. A missing cleanup proves that it can find a bug. The helper and ownership-transfer cases test whether it can leave working code alone. Those cases matter just as much to someone deciding whether to keep the tool enabled.

## Beyond cleanup

These questions show up with files and context cancellation, but also with concurrent work. A function can have a way to wait for a goroutine and still skip that wait on an error path.

Locks bring a related problem: an operation can look fine until you compare it with another part of the program. A gohawk finding led to a [merged fix in Caddy](https://github.com/caddyserver/caddy/pull/7968), where an error path acquired two locks in the opposite order to another path. Neither path alone showed the conflict.

That's the kind of help I want from gohawk alongside existing tests and analyzers. The [analyzer catalog](/analyzers/) describes the checks and their limits, if you want to see which ones apply to your code.

You can find [gohawk on GitHub](https://github.com/kojah/gohawk). If you try it, I'd like to hear what it finds. And if it complains because you had the audacity to write a helper function, I'd especially like to hear about that.
