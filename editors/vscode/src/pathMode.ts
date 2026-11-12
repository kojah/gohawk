// Path stepping: walks the locations a finding reports and dims everything
// else. It only navigates. Editing a file on the path ends the mode, because
// the positions may no longer be right.

import * as vscode from 'vscode';

export type Step = { uri: vscode.Uri; range: vscode.Range; message: string };

/**
 * The finding's own location and its related locations, in source order
 * within each file. A related location at the finding's own range replaces
 * the generic "reported here" step.
 */
export function stepsOf(uri: vscode.Uri, diagnostic: vscode.Diagnostic): Step[] {
	const related: Step[] = (diagnostic.relatedInformation ?? []).map((info) => ({
		uri: info.location.uri,
		range: info.location.range,
		message: info.message,
	}));
	const repeatsFinding = related.some(
		(step) => step.uri.toString() === uri.toString() && step.range.isEqual(diagnostic.range),
	);
	const own: Step[] = repeatsFinding
		? []
		: [{ uri, range: diagnostic.range, message: 'reported here' }];
	return [...own, ...related].sort((a, b) =>
		a.uri.toString() === b.uri.toString() ? a.range.start.compareTo(b.range.start) : 0,
	);
}

/** Every run of lines that holds no step. */
function offPathLines(
	document: vscode.TextDocument,
	ranges: readonly vscode.Range[],
): vscode.Range[] {
	const onPath = new Set<number>();
	for (const range of ranges) {
		for (let line = range.start.line; line <= range.end.line; line++) {
			onPath.add(line);
		}
	}
	const result: vscode.Range[] = [];
	let start: number | undefined;
	for (let line = 0; line <= document.lineCount; line++) {
		const off = line < document.lineCount && !onPath.has(line);
		if (off && start === undefined) {
			start = line;
		} else if (!off && start !== undefined) {
			result.push(new vscode.Range(start, 0, line - 1, document.lineAt(line - 1).text.length));
			start = undefined;
		}
	}
	return result;
}

export class PathStepper implements vscode.Disposable {
	private steps: readonly Step[] = [];
	private index = 0;
	private title = '';
	private readonly dim = vscode.window.createTextEditorDecorationType({ opacity: '0.4' });
	private readonly onPath = vscode.window.createTextEditorDecorationType({
		backgroundColor: new vscode.ThemeColor('editor.wordHighlightBackground'),
		borderRadius: '2px',
	});
	private readonly current = vscode.window.createTextEditorDecorationType({
		backgroundColor: new vscode.ThemeColor('editor.findMatchHighlightBackground'),
		border: '1px solid',
		borderColor: new vscode.ThemeColor('editorInfo.foreground'),
		borderRadius: '2px',
	});
	private readonly bar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 91);
	private readonly subscriptions: vscode.Disposable[];

	constructor() {
		this.bar.command = 'gohawk.stepNext';
		this.subscriptions = [
			vscode.workspace.onDidChangeTextDocument((event) => {
				const uri = event.document.uri.toString();
				if (
					event.contentChanges.length > 0 &&
					this.steps.some((step) => step.uri.toString() === uri)
				) {
					this.exit();
				}
			}),
			vscode.window.onDidChangeVisibleTextEditors(() => this.render()),
		];
	}

	get active(): boolean {
		return this.steps.length > 0;
	}

	async start(title: string, steps: readonly Step[]): Promise<void> {
		if (steps.length === 0) {
			return;
		}
		this.steps = steps;
		this.title = title;
		this.index = 0;
		await vscode.commands.executeCommand('setContext', 'gohawk.steppingPath', true);
		await this.reveal();
	}

	async move(delta: number): Promise<void> {
		if (!this.active) {
			return;
		}
		this.index = (this.index + delta + this.steps.length) % this.steps.length;
		await this.reveal();
	}

	exit(): void {
		if (!this.active) {
			return;
		}
		this.steps = [];
		this.bar.hide();
		for (const editor of vscode.window.visibleTextEditors) {
			this.clear(editor);
		}
		// Clearing a context key cannot fail in a way the user could act on.
		void vscode.commands.executeCommand('setContext', 'gohawk.steppingPath', false);
	}

	private clear(editor: vscode.TextEditor): void {
		editor.setDecorations(this.dim, []);
		editor.setDecorations(this.onPath, []);
		editor.setDecorations(this.current, []);
	}

	private async reveal(): Promise<void> {
		const step = this.steps[this.index];
		if (step === undefined) {
			return;
		}
		const document = await vscode.workspace.openTextDocument(step.uri);
		const editor = await vscode.window.showTextDocument(document, {
			selection: new vscode.Range(step.range.start, step.range.start),
		});
		editor.revealRange(step.range, vscode.TextEditorRevealType.InCenterIfOutsideViewport);
		this.bar.text = `$(debug-step-over) gohawk: path ${this.index + 1}/${this.steps.length} · ${step.message}`;
		this.bar.tooltip = `${this.title}\n\nAlt+] next · Alt+[ previous · Esc to exit`;
		this.bar.show();
		this.render();
	}

	private render(): void {
		if (!this.active) {
			return;
		}
		for (const editor of vscode.window.visibleTextEditors) {
			const uri = editor.document.uri.toString();
			const here = this.steps
				.map((step, index) => ({ step, index }))
				.filter(({ step }) => step.uri.toString() === uri);
			if (here.length === 0) {
				this.clear(editor);
				continue;
			}
			const label = (entry: { step: Step; index: number }): vscode.DecorationOptions => ({
				range: entry.step.range,
				renderOptions: {
					after: {
						contentText: `  ${entry.index + 1} · ${entry.step.message}`,
						color: new vscode.ThemeColor(
							entry.index === this.index ? 'editorInfo.foreground' : 'editorCodeLens.foreground',
						),
						fontStyle: 'normal',
					},
				},
			});
			editor.setDecorations(
				this.current,
				here.filter(({ index }) => index === this.index).map(label),
			);
			editor.setDecorations(
				this.onPath,
				here.filter(({ index }) => index !== this.index).map(label),
			);
			editor.setDecorations(
				this.dim,
				offPathLines(
					editor.document,
					here.map(({ step }) => step.range),
				),
			);
		}
	}

	dispose(): void {
		this.exit();
		this.dim.dispose();
		this.onPath.dispose();
		this.current.dispose();
		this.bar.dispose();
		for (const subscription of this.subscriptions) {
			subscription.dispose();
		}
	}
}
