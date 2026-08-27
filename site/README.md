# gohawk documentation site

The Starlight site reads its pages directly from `../docs`.

```sh
pnpm install
pnpm dev
pnpm build
```

Analyzer navigation, index tables, option tables, and rule metadata come from
the registered Go analyzers. From the repository root, refresh them with:

```sh
go generate ./...
```

`pnpm check` verifies that the generated files and handwritten analyzer
pages are in sync, then type-checks the site with Astro and TypeScript.
