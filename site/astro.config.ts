import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import analyzerManifest from './src/generated/analyzers.json' with { type: 'json' };
import { pluginGohawkDiagnostics } from './src/plugins/gohawk-diagnostics.ts';

const analyzerSidebar = [
	{ label: 'Overview', slug: 'analyzers' },
	...analyzerManifest.groups.map((group) => ({
		label: group.title,
		items: group.analyzers.map((analyzer) => ({
			label: analyzer.name,
			slug: analyzer.path,
		})),
	})),
];

export default defineConfig({
	site: 'https://kojah.github.io',
	base: '/gohawk/',
	redirects: {
		'/analyzer-reference': '/gohawk/analyzers/',
	},
	integrations: [
		starlight({
			title: 'gohawk',
			description: 'High-signal static analysis for Go.',
			customCss: ['./src/styles/field-manual.css'],
			components: {
				Head: './src/components/Head.astro',
				PageTitle: './src/components/PageTitle.astro',
			},
			editLink: {
				baseUrl: 'https://github.com/kojah/gohawk/edit/main/docs/',
			},
			expressiveCode: {
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
				{ label: 'Introduction', slug: 'index' },
				{
					label: 'Using gohawk',
					items: [
						{ label: 'Configuration and suppressions', slug: 'configuration' },
					],
				},
				{
					label: 'Analyzers',
					items: analyzerSidebar,
				},
				{ label: 'Contributing', slug: 'contributing' },
			],
		}),
	],
});
