import * as esbuild from 'esbuild';

const shared = {
	bundle: true,
	external: ['vscode', 'mocha'],
	format: 'cjs',
	platform: 'node',
	target: 'node20',
	sourcemap: true,
};

await esbuild.build({ ...shared, entryPoints: ['src/extension.ts'], outfile: 'dist/extension.js' });

// The editor tests run inside a real VS Code, which loads CommonJS files.
if (process.argv.includes('--tests')) {
	await esbuild.build({
		...shared,
		entryPoints: ['test/editor/runTest.ts', 'test/editor/index.ts', 'test/editor/findings.test.ts'],
		outdir: 'out/test',
	});
}
