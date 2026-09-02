import react from '@astrojs/react';
import starlight from '@astrojs/starlight';
import { defineConfig } from 'astro/config';
import analyzerManifest from './src/generated/analyzers.json' with { type: 'json' };
import { pluginGohawkDiagnostics } from './src/plugins/gohawk-diagnostics.ts';

// Each analyzer group is a label and its pages, nested under the 'Analyzer
// reference' section alongside the catalog overview.
const analyzerSidebar = analyzerManifest.groups.map((group) => ({
	label: group.title,
	// Omit item labels so Starlight derives them from the same frontmatter title
	// used for the page heading. This keeps sidebar labels and <h1>s identical.
	items: group.analyzers.map((analyzer) => ({ slug: analyzer.path })),
}));

const isDevelopment = process.env.NODE_ENV === 'development';
const viteCacheDir = process.env.GOHAWK_ASTRO_CHECK
	? 'node_modules/.vite-check'
	: isDevelopment
		? 'node_modules/.vite-dev'
		: 'node_modules/.vite-build';

export default defineConfig({
	site: 'https://gohawk.dev',
	base: '/',
	vite: {
		cacheDir: viteCacheDir,
		server: {
			// The docs collection lives outside the Astro project root. Polling keeps
			// hot reload reliable when an editor or Git replaces a Markdown file
			// atomically instead of emitting an ordinary in-place change event.
			watch: {
				usePolling: true,
				interval: 300,
			},
			proxy: {
				'/__agentation-review': {
					target: 'http://127.0.0.1:4848',
					rewrite: (path) => path.replace(/^\/__agentation-review/, ''),
				},
				'/__agentation': {
					target: 'http://127.0.0.1:4747',
					rewrite: (path) => path.replace(/^\/__agentation/, ''),
				},
			},
		},
	},
	redirects: {
		'/analyzer-reference': '/analyzers/',
		// The per-group index pages were folded into the catalog.
		'/analyzers/api-and-data-contracts': '/analyzers/',
		'/analyzers/ownership-and-lifecycle': '/analyzers/',
		'/analyzers/reliability-and-safety': '/analyzers/',
		'/analyzers/testing': '/analyzers/',
		'/tags-and-profiles': '/configuration/#choose-what-runs',
	},
	integrations: [
		...(isDevelopment ? [react()] : []),
		starlight({
			title: 'gohawk',
			description: 'High-signal static analysis for Go.',
			customCss: ['./src/styles/field-manual.css'],
			components: {
				Footer: isDevelopment
					? './src/components/Footer.astro'
					: './src/components/PaginationFooter.astro',
				Head: './src/components/Head.astro',
				PageTitle: './src/components/PageTitle.astro',
				ThemeSelect: './src/components/ThemeToggle.astro',
			},
			editLink: {
				baseUrl: 'https://github.com/kojah/gohawk/edit/main/docs/',
			},
			expressiveCode: {
				emitExternalStylesheet: false,
				themes: ['github-dark', 'github-light'],
				plugins: [pluginGohawkDiagnostics()],
				styleOverrides: {
					borderRadius: '6px',
					frames: { shadowColor: 'transparent' },
				},
			},
			lastUpdated: true,
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/kojah/gohawk',
				},
			],
			sidebar: [
				{
					label: 'Getting started',
					items: [
						{ slug: 'installation' },
						{ slug: 'configuration' },
						{ slug: 'check-tiers' },
						{ slug: 'golangci-lint' },
					],
				},
				{
					label: 'Contributing',
					items: [
						{ slug: 'contributing' },
						// The development references are contributor material, so they sit
						// inside Contributing rather than beside it. This is the same
						// group-within-a-section shape the analyzer groups use.
						{
							label: 'Development',
							items: [
								// What the analysis is built out of comes first: the layers a run
								// moves through, then what a cross-package fact may claim. The
								// pages below answer questions that arise while writing a check.
								{
									label: 'Architecture',
									items: [
										{ slug: 'architecture' },
										{ slug: 'development/understanding-ssa' },
										{ slug: 'development/fact-model' },
									],
								},
								{ slug: 'development/debugging-reference' },
							],
						},
						// Policy for contributors, so it closes the section rather than
						// sitting among the development references.
						{ slug: 'ai-policy' },
					],
				},
				{
					label: 'Analyzer reference',
					items: [{ slug: 'analyzers' }, ...analyzerSidebar],
				},
			],
		}),
	],
});
