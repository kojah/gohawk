# What I learned building gohawk

*Introduce gohawk through what I wanted to catch: resource-management and concurrency bugs that are easy to overlook in otherwise reasonable Go code.*

## Where this fits in Go’s tooling

*Briefly explain what existing tools contribute and where I wanted gohawk to help. Focus on specific capabilities, without claiming nobody else addresses these problems.*

## A missing wait in Docker

*Walk through the finding with a short code example. Show why having a Wait call isn’t enough, then explain how the fix removed manual process management.*

## I thought SSA would be enough

*Describe my initial expectation and the unexpected need to build another layer. Use cleanup inside a helper to explain what the fact system adds, saving implementation details for a follow-up.*

## More than 500 repositories later

*Describe scanning real projects and still finding false positives. Explain how ordinary, valid code challenged our assumptions—even after building a richer analysis model.*

## How many exceptions are too many?

*Discuss considering whether to disable globalstate entirely. Show how accumulating exceptions made me question the approach, without inventing a neat resolution.*

## What makes it worth continuing

*Bring in the Caddy fix as another concrete result. End with what I want to keep working on and an invitation to try gohawk—including reporting what it gets wrong.*
