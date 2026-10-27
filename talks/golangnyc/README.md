# Resource ownership in Go

A Slidev talk for Go developers: 50 minutes of presentation and 10 minutes of
questions. There are 37 main slides and three backup slides. Every slide has
speaker notes. The opening file leak connects the examples, analysis model,
precision discussion and closing question.

## Present

Requires Node.js 22.12 or later, pnpm, and Go 1.26 or later for example validation.
The deck uses Slidev's Vue runtime; it does not add another application framework.

```sh
cd talks/golangnyc
pnpm install --frozen-lockfile
pnpm dev
```

Open the local URL printed by Slidev. Press `p` for presenter mode, or open
`http://localhost:3030/presenter/1`. Speaker notes contain section timings,
audience prompts, transitions, caveats and sources. Fonts and the logo are
bundled locally; presenting the built deck does not require Google Fonts or a CDN.

## Rehearsal map

| Slides | Section                           | Time      | Elapsed |
| ------ | --------------------------------- | --------- | ------- |
| 1–7    | Spot the bug                      | 8 min     | 08:00   |
| 8–14   | What gohawk catches               | 10 min    | 18:00   |
| 15–23  | How it follows the code           | 15 min    | 33:00   |
| 24–30  | When it should stay silent        | 10 min    | 43:00   |
| 31–36  | Docker finding and adoption       | 7 min     | 50:00   |
| 37     | Questions                         | 10 min    | 60:00   |
| 38–40  | Backup: scope, processes, sources | As needed | —       |

`flow.dot` is the editable Bobby plan. `public/talk-flow.svg` is its rendering.
Re-render it with Bobby's `diagram flow.dot --svg public/talk-flow.svg` command.
The plan is a proposed narrative, not an extracted code dependency graph.

## Build, check and export

On Xenia, prefix heavy commands with `/home/james/.local/bin/agent-test`.

```sh
pnpm lint
pnpm build
pnpm exec playwright install chromium --only-shell
pnpm preview
# In another terminal, from this directory:
pnpm check
pnpm export
pnpm export:notes
```

`check` opens every built slide in Chromium, checks browser errors, missing images,
and content outside the canvas, then saves screenshots to `dist/shots/` and a
report to `dist/slide-check.json`. Inspect screenshots as well as the report.
`SLIDEV_CHECK_URL` overrides the preview URL (default `http://127.0.0.1:4173`).
PDFs are written under `dist/`. Use `pnpm export --range 1-37` to omit backups.
The deck has no custom TypeScript or Vue components needing a separate type check.
Browser checks and example analysis validate the actual rendered/code artifacts;
there is no unit-test suite for slide text or styling.

## Validate the examples

The slides import named regions directly from `examples/*.go`. That separate Go
module contains deliberate defects alongside accepted forms. Compile and analyze
it; do not execute its subprocess or callback examples as a program.

From the repository root:

```sh
agent-test go build -o .build/golangnyc-gohawk .
cd talks/golangnyc/examples
GOWORK=off agent-test go test ./...
GOWORK=off agent-test ../../../.build/golangnyc-gohawk \
  -enable=resourcelifetime,goroutineownership \
  -enable-checks=processownership/missing-wait -json ./...
```

The analyzer exits 3 for the four intentional findings. Expect exactly:

| Function      | Check                              |
| ------------- | ---------------------------------- |
| `printHeader` | `resourcelifetime/missing-release` |
| `openValidated` | `resourcelifetime/missing-release` |
| `refresh`     | `goroutineownership/unjoined`      |
| `run`         | `processownership/missing-wait`    |

Expect no diagnostics in `printHeaderSafely`, `printViaHelper`, `openHeader`,
`openValidatedSafely`, `refreshSafely` or `runSafely`. The read-only examples
deliberately ignore secondary Close errors; code that writes or finalizes data may need to retain
that error. Selecting the experimental check by ID is required: naming only its
analyzer selects core checks and will not run `missing-wait`.

## Sources and claim boundaries

Drafted against gohawk revision `07b498eef6198daee85ca4f39000285a0e0cc876`:

- [Run model and lifecycle structure](https://github.com/kojah/gohawk/blob/07b498eef6198daee85ca4f39000285a0e0cc876/docs/architecture.md)
- [Function facts and argument binding](https://github.com/kojah/gohawk/blob/07b498eef6198daee85ca4f39000285a0e0cc876/docs/development/fact-model.md)
- [Resource lifetime contracts](https://github.com/kojah/gohawk/blob/07b498eef6198daee85ca4f39000285a0e0cc876/docs/analyzers/resources-and-lifecycle/resourcelifetime.mdx)
- [Goroutine ownership boundaries](https://github.com/kojah/gohawk/blob/07b498eef6198daee85ca4f39000285a0e0cc876/docs/analyzers/concurrency-and-synchronization/goroutineownership.mdx)
- [Experimental process ownership](https://github.com/kojah/gohawk/blob/07b498eef6198daee85ca4f39000285a0e0cc876/docs/analyzers/resources-and-lifecycle/processownership.mdx)
- [Docker PR #53517, including reproduction and discovery disclosure](https://github.com/moby/moby/pull/53517)
- [Exact Docker patch](https://github.com/moby/moby/commit/4070d0ac379e4b5adde89e41f6d94d202630d960)
- [os/exec lifecycle and stream handling](https://pkg.go.dev/os/exec#Cmd)

The Docker section distinguishes the missing-Wait finding from the pipe deadlock
found during review. It describes the patch without claiming a current merge
status. The SSA example and summary diagrams are teaching models, not literal
internal schemas. There are no benchmark numbers or corpus precision rates.
Revalidate code and links before presenting a later version.

This deck and its examples were prepared with AI assistance and checked against
the repository documentation, the analyzer and the cited patch.
