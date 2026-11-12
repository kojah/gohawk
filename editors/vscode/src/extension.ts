// Entry point. The extension is a thin client: it finds the gohawk binary,
// starts `gohawk lsp`, and adds the three things the protocol does not give
// for free: the status bar item, path stepping, and the explanation tab.

import * as vscode from 'vscode';
import { ExplainView } from './explain.ts';
import { findBinary, offerDownload } from './install.ts';
import { PathStepper, stepsOf } from './pathMode.ts';
import { type FindingTarget, parseFindingTarget } from './protocol.ts';
import { errorMessage } from './release.ts';
import { Session } from './session.ts';
import { StatusItem } from './status.ts';

const stepPathCommand = 'gohawk.stepPath';
const explainCommand = 'gohawk.explainFinding';
const source = 'gohawk';

type Finding = { uri: vscode.Uri; diagnostic: vscode.Diagnostic };

function checkOf(diagnostic: vscode.Diagnostic): string {
	const code = diagnostic.code;
	if (typeof code === 'object') {
		return String(code.value);
	}
	return code === undefined ? '' : String(code);
}

function gohawkFindings(uri: vscode.Uri): vscode.Diagnostic[] {
	return vscode.languages.getDiagnostics(uri).filter((diagnostic) => diagnostic.source === source);
}

/** The gohawk finding under the cursor, or the first one in the file. */
function findingAtCursor(): Finding | undefined {
	const editor = vscode.window.activeTextEditor;
	if (editor === undefined) {
		return undefined;
	}
	const findings = gohawkFindings(editor.document.uri);
	const line = editor.selection.active.line;
	const diagnostic =
		findings.find(
			(finding) => finding.range.start.line <= line && line <= finding.range.end.line,
		) ?? findings[0];
	return diagnostic === undefined ? undefined : { uri: editor.document.uri, diagnostic };
}

/** The finding a server code action named, among the diagnostics the editor holds. */
function findingFor(target: FindingTarget): Finding | undefined {
	const uri = vscode.Uri.parse(target.uri);
	const diagnostic = gohawkFindings(uri).find(
		(finding) =>
			finding.range.start.line === target.line - 1 &&
			finding.range.start.character === target.character &&
			checkOf(finding) === target.check,
	);
	return diagnostic === undefined ? undefined : { uri, diagnostic };
}

function targetOf(finding: Finding): FindingTarget {
	const check = checkOf(finding.diagnostic);
	return {
		uri: finding.uri.toString(),
		line: finding.diagnostic.range.start.line + 1,
		character: finding.diagnostic.range.start.character,
		analyzer: check.split('/')[0] ?? check,
		check,
	};
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
	const output = vscode.window.createOutputChannel('gohawk');
	const status = new StatusItem();
	const stepper = new PathStepper();
	const explain = new ExplainView();
	const session = new Session({
		output,
		clientCommands: [stepPathCommand, explainCommand],
		readSettings: () => vscode.workspace.getConfiguration('gohawk'),
		onStatus: (next) => status.show(next),
	});
	context.subscriptions.push(output, status, stepper, explain, session);

	const showStatusForGo = () =>
		status.setVisible(
			vscode.window.activeTextEditor?.document.languageId === 'go' || stepper.active,
		);
	showStatusForGo();
	context.subscriptions.push(vscode.window.onDidChangeActiveTextEditor(showStatusForGo));

	/** Reports a failed command instead of losing the rejection. */
	const guarded =
		<Arguments extends unknown[]>(action: (...argumentList: Arguments) => Promise<void>) =>
		async (...argumentList: Arguments): Promise<void> => {
			try {
				await action(...argumentList);
			} catch (error) {
				output.appendLine(`error: ${errorMessage(error)}`);
				await vscode.window.showErrorMessage(`gohawk: ${errorMessage(error)}`);
			}
		};

	const launch = guarded(async () => {
		await session.stop();
		const notInstalled = (tooltip: string) =>
			status.show({ state: 'skipped', text: 'gohawk: not installed', tooltip, findings: 0 });
		let binary = await findBinary(context);
		if (binary === undefined) {
			notInstalled('The gohawk binary was not found.');
			binary = await offerDownload(context);
		}
		if (binary === undefined) {
			notInstalled('Set gohawk.path, or run “gohawk: Restart Server” to try again.');
			return;
		}
		try {
			await session.start(binary);
		} catch (error) {
			status.show({
				state: 'error',
				text: 'gohawk: failed to start',
				tooltip: errorMessage(error),
				findings: 0,
			});
			throw error;
		}
	});

	const explainFinding = async (target: FindingTarget) => {
		const text = await vscode.window.withProgress(
			{ location: vscode.ProgressLocation.Window, title: 'gohawk: explaining finding…' },
			() => session.command('gohawk.server.explain', target),
		);
		if (typeof text !== 'string') {
			throw new Error('the server returned no explanation');
		}
		await explain.show(`${target.check.replace('/', ' ')} line ${target.line}`, text);
	};

	const stepThrough = async (finding: Finding | undefined) => {
		if (finding === undefined) {
			await vscode.window.showInformationMessage('No gohawk finding here.');
			return;
		}
		await stepper.start(finding.diagnostic.message, stepsOf(finding.uri, finding.diagnostic));
		showStatusForGo();
	};

	const activeUri = () => vscode.window.activeTextEditor?.document.uri.toString();
	const register = <Arguments extends unknown[]>(
		name: string,
		action: (...argumentList: Arguments) => Promise<void>,
	) => context.subscriptions.push(vscode.commands.registerCommand(name, guarded(action)));

	register('gohawk.checkPackage', async () => {
		await session.command('gohawk.server.checkPackage', activeUri());
	});
	register('gohawk.checkWorkspace', async () => {
		await session.command('gohawk.server.checkWorkspace', activeUri());
	});
	register('gohawk.clearResults', async () => {
		await session.command('gohawk.server.clearResults');
	});
	register('gohawk.showOutput', async () => output.show(true));
	register('gohawk.showProblems', async () => {
		await vscode.commands.executeCommand('workbench.actions.view.problems');
	});
	register('gohawk.restart', launch);
	register('gohawk.stepNext', () => stepper.move(1));
	register('gohawk.stepPrevious', () => stepper.move(-1));
	register('gohawk.stepExit', async () => {
		stepper.exit();
		showStatusForGo();
	});
	register('gohawk.stepPathAtCursor', () => stepThrough(findingAtCursor()));
	register('gohawk.explainAtCursor', async () => {
		const finding = findingAtCursor();
		if (finding === undefined) {
			await vscode.window.showInformationMessage('No gohawk finding here.');
			return;
		}
		await explainFinding(targetOf(finding));
	});

	// The server's code actions invoke these two with a finding target. The
	// argument crosses a process boundary, so it is parsed before use.
	register(stepPathCommand, async (argument: unknown) => {
		const target = parseFindingTarget(argument);
		await stepThrough(target === undefined ? undefined : findingFor(target));
	});
	register(explainCommand, async (argument: unknown) => {
		const target = parseFindingTarget(argument);
		if (target === undefined) {
			throw new Error('the finding to explain was not named');
		}
		await explainFinding(target);
	});

	context.subscriptions.push(
		vscode.workspace.onDidChangeConfiguration((event) => {
			if (
				event.affectsConfiguration('gohawk.path') ||
				event.affectsConfiguration('gohawk.eventLog')
			) {
				// launch reports its own failures; nothing is left to reject.
				void launch();
			}
		}),
	);

	await launch();
}
