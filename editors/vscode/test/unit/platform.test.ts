import assert from 'node:assert/strict';
import { test } from 'node:test';
import { executableName, platformForTarget, platformOf, platforms } from '../../src/platform.ts';

test('every release target of gohawk has a package target', () => {
	const built = platforms.map((platform) => `${platform.go.os}/${platform.go.arch}`).sort();
	assert.deepEqual(built, [
		'darwin/amd64',
		'darwin/arm64',
		'linux/amd64',
		'linux/arm64',
		'windows/amd64',
		'windows/arm64',
	]);
	assert.equal(new Set(platforms.map((platform) => platform.vsce)).size, platforms.length);
});

test('a running Node process maps to its package target', () => {
	assert.equal(platformOf('linux', 'x64')?.vsce, 'linux-x64');
	assert.equal(platformOf('darwin', 'arm64')?.go.arch, 'arm64');
	assert.equal(platformOf('win32', 'x64')?.go.os, 'windows');
	assert.equal(platformOf('freebsd', 'x64'), undefined);
	assert.equal(platformOf('linux', 'riscv64'), undefined);
});

test('a package target maps back to its platform', () => {
	assert.equal(platformForTarget('win32-arm64')?.go.arch, 'arm64');
	assert.equal(platformForTarget('alpine-x64'), undefined);
});

test('only Windows binaries carry an extension', () => {
	assert.equal(executableName('win32'), 'gohawk.exe');
	assert.equal(executableName('linux'), 'gohawk');
	assert.equal(executableName('darwin'), 'gohawk');
});
