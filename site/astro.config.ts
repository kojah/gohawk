import react from '@astrojs/react';
import starlight from '@astrojs/starlight';
import { defineConfig } from 'astro/config';
import analyzerManifest from './src/generated/analyzers.json' with { type: 'json' };
import { pluginGohawkDiagnostics } from './src/plugins/gohawk-diagnostics.ts';

// The sidebar is deliberately two levels deep: a label and its pages. Nesting
// the analyzer groups under an 'Analyzers' parent pushed them to a third level,
// where Starlight indents them and stacks two disclosure carets.
const analyzerSidebar = analyzerManifest.groups.map((group) => ({
	label: group.title,
	items: group.analyzers.map((analyzer) => ({
		label: analyzer.name,
		slug: analyzer.path,
	})),
}));

const isDevelopment = process.env.NODE_ENV === 'development';
const viteCacheDir = process.env.GOHAWK_ASTRO_CHECK
	? 'node_modules/.vite-check'
	: isDevelopment
		? 'node_modules/.vite-dev'
		: 'node_modules/.vite-build';

export default defineConfig({
	site: 'https://kojah.github.io',
	base: '/gohawk/',
	vite: {
		cacheDir: viteCacheDir,
		server: {
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
		'/analyzer-reference': '/gohawk/analyzers/',
		// The per-group index pages were folded into the catalog.
		'/analyzers/api-and-data-contracts': '/gohawk/analyzers/',
		'/analyzers/ownership-and-lifecycle': '/gohawk/analyzers/',
		'/analyzers/reliability-and-safety': '/gohawk/analyzers/',
		'/analyzers/testing': '/gohawk/analyzers/',
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
					: './src/components/EmptyFooter.astro',
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
						{ label: 'Installation', slug: 'installation' },
						{ label: 'golangci-lint', slug: 'golangci-lint' },
						{ label: 'Configuring gohawk', slug: 'configuration' },
						{ label: 'Tags and profiles', slug: 'tags-and-profiles' },
					],
				},
				{
					label: 'Contributing',
					items: [
						{ label: 'Overview', slug: 'contributing' },
						{ label: 'AI policy', slug: 'ai-policy' },
						{ label: 'Architecture', slug: 'architecture' },
					],
				},
				{
					label: 'Analyzer reference',
					items: [{ label: 'Overview', slug: 'analyzers' }, ...analyzerSidebar],
				},
			],
		}),
	],
});
