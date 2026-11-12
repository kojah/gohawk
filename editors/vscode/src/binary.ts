// Finds the gohawk binary. Each extension package carries the binary for its
// platform, so the normal answer is the bundled one. A configured path wins
// over it, for people who build gohawk themselves, and gohawk on PATH is the
// last resort for a package built without a binary.

import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import * as vscode from 'vscode';
import { executableName } from './platform.ts';

const run = promisify(execFile);
const probeTimeoutMs = 15_000;

export type Binary = { path: string; source: 'setting' | 'bundled' | 'PATH' };

/** Reports whether a binary has the lsp command; releases before 0.8 do not. */
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

export async function openPathSetting(): Promise<void> {
	await vscode.commands.executeCommand('workbench.action.openSettings', 'gohawk.path');
}

/**
 * Resolves a usable binary. A configured path that does not work is reported
 * and not silently replaced: the user asked for that binary.
 */
export async function findBinary(context: vscode.ExtensionContext): Promise<Binary | undefined> {
	const configured = vscode.workspace.getConfiguration('gohawk').get<string>('path', '').trim();
	if (configured !== '') {
		if (await supportsEditor(configured)) {
			return { path: configured, source: 'setting' };
		}
		const choice = await vscode.window.showErrorMessage(
			`The gohawk at gohawk.path (${configured}) cannot serve the editor. It is missing, or older than 0.8.`,
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
	const name = executableName(process.platform);
	const bundled = vscode.Uri.joinPath(context.extensionUri, 'bin', name).fsPath;
	if (await supportsEditor(bundled)) {
		return { path: bundled, source: 'bundled' };
	}
	if (await supportsEditor(name)) {
		return { path: name, source: 'PATH' };
	}
	return undefined;
}
