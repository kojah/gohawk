import * as esbuild from 'esbuild';

await esbuild.build({
	entryPoints: ['src/extension.ts'],
	outfile: 'dist/extension.js',
	bundle: true,
	external: ['vscode'],
	format: 'cjs',
	platform: 'node',
	target: 'node20',
	sourcemap: true,
});
