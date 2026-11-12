// Finds the gohawk binary: the configured path, then the version this
// extension downloaded, then gohawk on PATH. A download is the prebuilt
// release archive for this platform, checked against the published checksums
// before anything is unpacked.

import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import * as fs from 'node:fs/promises';
import * as os from 'node:os';
import * as path from 'node:path';
import { promisify } from 'node:util';
import * as vscode from 'vscode';
import {
	errorMessage,
	executableName,
	expectedChecksum,
	pinnedVersion,
	releaseTarget,
} from './release.ts';

const run = promisify(execFile);
const releaseBase = 'https://github.com/kojah/gohawk/releases/download';
const probeTimeoutMs = 15_000;
const downloadTimeoutMs = 120_000;

export type Binary = { path: string; source: 'setting' | 'download' | 'PATH' };

/** Reports whether a binary has the lsp command; older releases do not. */
async function supportsEditor(binary: string): Promise<boolean> {
	try {
		const { stderr } = await run(binary, ['lsp', '--help'], { timeout: probeTimeoutMs });
		return stderr.includes('usage: gohawk lsp');
	} catch {
		// A missing file, a refusal to execute, and an old release that treats
		// "lsp" as a package pattern all mean the same thing here: not usable.
		return false;
	}
}

function downloadedPath(context: vscode.ExtensionContext): string {
	return path.join(
		context.globalStorageUri.fsPath,
		pinnedVersion,
		executableName(process.platform),
	);
}

async function openPathSetting(): Promise<void> {
	await vscode.commands.executeCommand('workbench.action.openSettings', 'gohawk.path');
}

/** Resolves a usable binary without downloading anything. */
export async function findBinary(context: vscode.ExtensionContext): Promise<Binary | undefined> {
	const configured = vscode.workspace.getConfiguration('gohawk').get<string>('path', '').trim();
	if (configured !== '') {
		if (await supportsEditor(configured)) {
			return { path: configured, source: 'setting' };
		}
		const choice = await vscode.window.showErrorMessage(
			`The gohawk at gohawk.path (${configured}) does not support editor integration. Version ${pinnedVersion} or newer is needed.`,
			'Open Settings',
			'Use Bundled Version',
		);
		if (choice === 'Open Settings') {
			await openPathSetting();
		}
		if (choice !== 'Use Bundled Version') {
			return undefined;
		}
	}
	const downloaded = downloadedPath(context);
	if (await supportsEditor(downloaded)) {
		return { path: downloaded, source: 'download' };
	}
	const onPath = executableName(process.platform);
	if (await supportsEditor(onPath)) {
		return { path: onPath, source: 'PATH' };
	}
	return undefined;
}

async function fetchBytes(url: string, signal: AbortSignal): Promise<Buffer> {
	const response = await fetch(url, { redirect: 'follow', signal });
	if (!response.ok) {
		throw new Error(`${url}: HTTP ${response.status}`);
	}
	return Buffer.from(await response.arrayBuffer());
}

/** Downloads, verifies and unpacks the pinned release. Throws with a reason on any failure. */
async function downloadBinary(
	context: vscode.ExtensionContext,
	signal: AbortSignal,
): Promise<Binary> {
	const target = releaseTarget(pinnedVersion, process.platform, process.arch);
	if (target === undefined) {
		throw new Error(
			`there is no prebuilt gohawk for ${process.platform}/${process.arch}; install it with "go install github.com/kojah/gohawk@latest" and set gohawk.path`,
		);
	}
	const base = `${releaseBase}/v${pinnedVersion}`;
	const [archive, checksums] = await Promise.all([
		fetchBytes(`${base}/${target.archive}`, signal),
		fetchBytes(`${base}/checksums.txt`, signal),
	]);
	const expected = expectedChecksum(checksums.toString('utf8'), target.archive);
	const actual = createHash('sha256').update(archive).digest('hex');
	if (expected === undefined || expected !== actual) {
		throw new Error(`the checksum of ${target.archive} does not match; nothing was installed`);
	}

	const staging = await fs.mkdtemp(path.join(os.tmpdir(), 'gohawk-download-'));
	try {
		const archivePath = path.join(staging, target.archive);
		await fs.writeFile(archivePath, archive);
		// tar reads both formats here: bsdtar on Windows and macOS unpacks zip,
		// and Linux releases are tar.gz.
		await run('tar', ['-xf', archivePath, '-C', staging], { signal });
		const destination = downloadedPath(context);
		await fs.mkdir(path.dirname(destination), { recursive: true });
		await fs.copyFile(path.join(staging, target.name, target.executable), destination);
		await fs.chmod(destination, 0o755);
		if (!(await supportsEditor(destination))) {
			throw new Error(`gohawk ${pinnedVersion} was installed but did not start`);
		}
		return { path: destination, source: 'download' };
	} finally {
		await fs.rm(staging, { recursive: true, force: true });
	}
}

/** Asks once whether to download, and reports why when it cannot. */
export async function offerDownload(context: vscode.ExtensionContext): Promise<Binary | undefined> {
	const choice = await vscode.window.showInformationMessage(
		`gohawk needs its analyzer binary. Download gohawk v${pinnedVersion} for ${process.platform}/${process.arch}?`,
		'Download',
		'Set Path…',
	);
	if (choice === 'Set Path…') {
		await openPathSetting();
	}
	if (choice !== 'Download') {
		return undefined;
	}
	try {
		return await vscode.window.withProgress(
			{
				location: vscode.ProgressLocation.Notification,
				title: `Downloading gohawk v${pinnedVersion}…`,
				cancellable: true,
			},
			(_progress, token) => {
				const cancel = new AbortController();
				token.onCancellationRequested(() => cancel.abort());
				const signal = AbortSignal.any([cancel.signal, AbortSignal.timeout(downloadTimeoutMs)]);
				return downloadBinary(context, signal);
			},
		);
	} catch (error) {
		const next = await vscode.window.showErrorMessage(
			`gohawk could not be downloaded: ${errorMessage(error)}`,
			'Set Path…',
		);
		if (next === 'Set Path…') {
			await openPathSetting();
		}
		return undefined;
	}
}
