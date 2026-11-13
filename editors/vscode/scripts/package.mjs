// Builds one extension package per platform, each carrying the gohawk binary
// for that platform in bin/.
//
//   node scripts/package.mjs                      the platform this runs on
//   node scripts/package.mjs --target darwin-arm64
//   node scripts/package.mjs --all
//   node scripts/package.mjs --target linux-x64 --binary /path/to/gohawk
//
// Without --binary the gohawk binary is cross-compiled from this repository.
// --version sets the package version without editing package.json; a release
// passes the gohawk version so the two always match.

import { execFile } from 'node:child_process';
import { chmod, copyFile, mkdir, rm } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs, promisify } from 'node:util';
import { executableName, platformForTarget, platformOf, platforms } from '../src/platform.ts';

const run = promisify(execFile);
const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = resolve(packageRoot, '..', '..');

const { values: options } = parseArgs({
	options: {
		target: { type: 'string' },
		all: { type: 'boolean', default: false },
		binary: { type: 'string' },
		version: { type: 'string' },
		out: { type: 'string', default: packageRoot },
	},
});

function fail(message) {
	console.error(`package: ${message}`);
	process.exit(2);
}

function selectedPlatforms() {
	if (options.all) {
		if (options.binary) {
			fail('--binary names one binary, so it cannot be combined with --all');
		}
		return platforms;
	}
	const platform = options.target
		? platformForTarget(options.target)
		: platformOf(process.platform, process.arch);
	if (!platform) {
		fail(
			`no package is built for ${options.target ?? `${process.platform}/${process.arch}`}; targets: ${platforms.map((p) => p.vsce).join(', ')}`,
		);
	}
	return [platform];
}

async function buildBinary(platform, destination) {
	const version = options.version ? `v${options.version.replace(/^v/, '')}` : 'devel';
	await run(
		'go',
		[
			'build',
			'-trimpath',
			`-ldflags=-s -w -X=github.com/kojah/gohawk/internal/cli.buildVersion=${version}`,
			'-o',
			destination,
			'.',
		],
		{
			cwd: repositoryRoot,
			env: { ...process.env, GOOS: platform.go.os, GOARCH: platform.go.arch, CGO_ENABLED: '0' },
		},
	);
}

async function packageFor(platform) {
	const bin = join(packageRoot, 'bin');
	await rm(bin, { recursive: true, force: true });
	await mkdir(bin, { recursive: true });
	const bundled = join(bin, executableName(platform.node.platform));
	if (options.binary) {
		await copyFile(resolve(options.binary), bundled);
	} else {
		await buildBinary(platform, bundled);
	}
	await chmod(bundled, 0o755);

	const version = options.version?.replace(/^v/, '');
	const name = `gohawk-${platform.vsce}${version ? `-${version}` : ''}.vsix`;
	const output = join(resolve(options.out), name);
	const versionArguments = version
		? [version, '--no-update-package-json', '--no-git-tag-version']
		: [];
	await run(
		'pnpm',
		[
			'exec',
			'vsce',
			'package',
			...versionArguments,
			'--target',
			platform.vsce,
			'--no-dependencies',
			// The listing page shows the README; its relative links and images
			// must point at this folder of the repository, not at its root.
			'--baseContentUrl',
			'https://github.com/kojah/gohawk/blob/main/editors/vscode',
			'--baseImagesUrl',
			'https://github.com/kojah/gohawk/raw/main/editors/vscode',
			'--out',
			output,
		],
		{ cwd: packageRoot },
	);
	console.log(output);
}

await run('node', ['esbuild.mjs'], { cwd: packageRoot });
await copyFile(join(repositoryRoot, 'LICENSE'), join(packageRoot, 'LICENSE'));
await mkdir(resolve(options.out), { recursive: true });
// One at a time: every package is assembled in the same bin directory.
for (const platform of selectedPlatforms()) {
	await packageFor(platform);
}
await rm(join(packageRoot, 'bin'), { recursive: true, force: true });
