# Developing the gohawk extension

```sh
pnpm install
pnpm check        # Biome and the TypeScript compiler
pnpm test         # unit tests for the pure modules
pnpm build        # bundle to dist/
pnpm package      # build a .vsix for this platform, with gohawk built from this repository
```

`node scripts/package.mjs --all` builds the six platform packages, each with its own gohawk binary.

Press F5 in this folder to try a change in an Extension Development Host. Set
`gohawk.path` there to a binary built from this repository (`go build -o
gohawk .` at the repository root).

## Acceptance run

```sh
pnpm acceptance --with-go-extension
```

This builds the package for this platform and installs it into a real VS Code served to a headless
browser, plays the scenarios in `acceptance/scenarios.mjs`, and writes
`out/acceptance/report.md` with screenshots, edit-to-findings latency, memory
per check, and anything that looks like a crash or a leftover process. It
needs a Chromium that Playwright can drive; set `ACCEPTANCE_CHROMIUM` to its
path if it is not in Playwright's cache. The process exits nonzero when a
check fails.

On Linux the run fetches Inter and JetBrains Mono once into `.vscode-test/`
and points the browser at them, so screenshots do not use a server's
fallback fonts. Nothing is installed on the machine.

## Stress run

```sh
node acceptance/stress.mjs --gohawk /path/to/gohawk --workspace /path/to/repo --file pkg/big/file.go
```

This drives `gohawk lsp` directly on a real package: it opens the file, makes
a few unsaved edits, and prints for each one how long the check took, its
peak memory, and what the importer run cost. The repository is never
modified.

## Publishing

A gohawk release attaches six `.vsix` files to its GitHub release, one per
platform, each carrying that release's binary. The `Publish extension`
workflow takes a release tag, downloads those files, checks them against the
release's checksums, and publishes them to the Visual Studio Marketplace and
to Open VSX, which is where Cursor and VSCodium install from.

One-time setup:

- **Marketplace.** Create the publisher `kojah` at
  <https://marketplace.visualstudio.com/manage>. On its Trusted Publishing
  page add a policy for the repository `kojah/gohawk` and the workflow file
  `publish-extension.yml`. The workflow then publishes with no stored secret.
- **Open VSX.** Sign in at <https://open-vsx.org> with GitHub, sign the
  publisher agreement, create the namespace `kojah`, and create an access
  token. Save it as the repository secret `OVSX_PAT`.

Then run the workflow from the Actions tab with the release tag. Either
registry can be switched off for a run. Publishing a version that is already
there is skipped, so a run can be repeated.
