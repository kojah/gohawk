// Runs inside the editor: opens the leaking file, waits for gohawk, tries an
// unsaved fix, and writes what it saw to GOHAWK_PROBE_OUT.
const vscode = require('vscode');
const fs = require('node:fs');
const path = require('node:path');
const sleep = (ms) => new Promise((done) => setTimeout(done, ms));
const gohawk = (uri) => vscode.languages.getDiagnostics(uri).filter((d) => d.source === 'gohawk');
async function until(check, ms) {
	const deadline = Date.now() + ms;
	while (Date.now() < deadline) {
		if (check()) return true;
		await sleep(250);
	}
	return false;
}
exports.run = async function run() {
	const result = { app: vscode.env.appName, vscodeVersion: vscode.version };
	try {
		const extension = vscode.extensions.getExtension('gohawk.gohawk');
		result.installed = extension !== undefined;
		result.extensionVersion = extension?.packageJSON.version;
		const folder = vscode.workspace.workspaceFolders[0].uri.fsPath;
		const document = await vscode.workspace.openTextDocument(
			path.join(folder, 'leak', 'header.go'),
		);
		await vscode.window.showTextDocument(document);
		await extension.activate();
		const started = Date.now();
		result.findingAppeared = await until(() => gohawk(document.uri).length > 0, 180000);
		result.secondsToFirstFinding = (Date.now() - started) / 1000;
		result.findings = gohawk(document.uri).map((d) => ({
			line: d.range.start.line + 1,
			check: typeof d.code === 'object' ? d.code.value : d.code,
			message: d.message,
			steps: (d.relatedInformation ?? []).map((r) => r.message),
		}));
		const first = gohawk(document.uri)[0];
		if (first) {
			const hovers = await vscode.commands.executeCommand(
				'vscode.executeHoverProvider',
				document.uri,
				first.range.start,
			);
			result.hoverHasHelp = JSON.stringify(
				hovers.map((h) => h.contents.map((c) => c.value ?? c)),
			).includes('gohawk help');
			const actions = await vscode.commands.executeCommand(
				'vscode.executeCodeActionProvider',
				document.uri,
				first.range,
			);
			result.actions = actions.map((a) => a.title).filter((t) => t.startsWith('gohawk'));
			// Another extension's welcome tab can take focus on first run.
			const editor = await vscode.window.showTextDocument(document);
			editor.selection = new vscode.Selection(first.range.start, first.range.start);
			await vscode.commands.executeCommand('gohawk.stepPathAtCursor');
			await sleep(500);
			await vscode.commands.executeCommand('gohawk.stepNext');
			await vscode.commands.executeCommand('gohawk.stepExit');
			result.pathSteppingRan = true;
		}
		const edit = new vscode.WorkspaceEdit();
		edit.insert(document.uri, new vscode.Position(14, 0), '\tdefer f.Close()\n');
		await vscode.workspace.applyEdit(edit);
		const editAt = Date.now();
		result.clearedByUnsavedFix = await until(() => gohawk(document.uri).length === 0, 90000);
		result.secondsToClear = (Date.now() - editAt) / 1000;
		result.stillUnsaved = document.isDirty;
		await vscode.commands.executeCommand('workbench.action.files.revert');
		result.findingBackAfterRevert = await until(() => gohawk(document.uri).length > 0, 90000);
	} catch (error) {
		result.error = String(error?.stack ?? error);
	}
	fs.writeFileSync(process.env.GOHAWK_PROBE_OUT, JSON.stringify(result, null, 1));
};
