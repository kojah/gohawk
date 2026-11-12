// Shows the evidence tracer's output for one finding in a read-only tab.

import * as vscode from 'vscode';

const scheme = 'gohawk-explain';

export class ExplainView implements vscode.TextDocumentContentProvider, vscode.Disposable {
	private readonly texts = new Map<string, string>();
	private readonly changed = new vscode.EventEmitter<vscode.Uri>();
	private readonly registration: vscode.Disposable;
	readonly onDidChange = this.changed.event;

	constructor() {
		this.registration = vscode.workspace.registerTextDocumentContentProvider(scheme, this);
	}

	provideTextDocumentContent(uri: vscode.Uri): string {
		return this.texts.get(uri.toString()) ?? '';
	}

	async show(title: string, text: string): Promise<void> {
		const uri = vscode.Uri.from({ scheme, path: `/${title.replace(/[\\/]/g, '-')}.txt` });
		this.texts.set(uri.toString(), text);
		this.changed.fire(uri);
		const document = await vscode.workspace.openTextDocument(uri);
		await vscode.window.showTextDocument(document, {
			preview: true,
			viewColumn: vscode.ViewColumn.Beside,
		});
	}

	dispose(): void {
		this.registration.dispose();
		this.changed.dispose();
		this.texts.clear();
	}
}
