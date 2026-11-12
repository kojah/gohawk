import assert from 'node:assert/strict';
import { test } from 'node:test';
import { expectedChecksum, releaseTarget } from '../../src/release.ts';

const digest = 'a'.repeat(64);

test('release target names the archive the release workflow publishes', () => {
	assert.deepEqual(releaseTarget('0.8.0', 'linux', 'x64'), {
		name: 'gohawk_0.8.0_linux_amd64',
		archive: 'gohawk_0.8.0_linux_amd64.tar.gz',
		executable: 'gohawk',
	});
	assert.deepEqual(releaseTarget('0.8.0', 'win32', 'arm64'), {
		name: 'gohawk_0.8.0_windows_arm64',
		archive: 'gohawk_0.8.0_windows_arm64.zip',
		executable: 'gohawk.exe',
	});
	assert.equal(
		releaseTarget('0.8.0', 'darwin', 'arm64')?.archive,
		'gohawk_0.8.0_darwin_arm64.tar.gz',
	);
});

test('a platform without a published archive has no target', () => {
	assert.equal(releaseTarget('0.8.0', 'freebsd', 'x64'), undefined);
	assert.equal(releaseTarget('0.8.0', 'linux', 'riscv64'), undefined);
});

test('expected checksum reads the line for the named archive only', () => {
	const checksums = `${'b'.repeat(64)}  gohawk_0.8.0_darwin_arm64.tar.gz\n${digest}  gohawk_0.8.0_linux_amd64.tar.gz\n`;
	assert.equal(expectedChecksum(checksums, 'gohawk_0.8.0_linux_amd64.tar.gz'), digest);
});

test('a missing or malformed checksum is not accepted', () => {
	assert.equal(expectedChecksum('', 'gohawk.tar.gz'), undefined);
	assert.equal(expectedChecksum(`${digest}  other.tar.gz`, 'gohawk.tar.gz'), undefined);
	assert.equal(expectedChecksum('not-a-digest  gohawk.tar.gz', 'gohawk.tar.gz'), undefined);
});
