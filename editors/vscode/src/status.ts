// The status bar item. The server composes the text; this file only picks
// an icon and a color for each state.

import * as vscode from 'vscode';
import type { Status, StatusState } from './protocol.ts';

const icons = {
	idle: '$(check)',
	findings: '$(warning)',
	checking: '$(sync~spin)',
	stale: '$(history)',
	stopped: '$(stop-circle)',
	error: '$(error)',
	skipped: '$(circle-slash)',
} as const satisfies Record<StatusState, string>;

const backgrounds: Partial<Record<StatusState, string>> = {
	stale: 'statusBarItem.warningBackground',
	stopped: 'statusBarItem.errorBackground',
	error: 'statusBarItem.errorBackground',
};

const defaultTooltip = 'Click to show gohawk findings in the Problems panel.';

export class StatusItem implements vscode.Disposable {
	private readonly item = vscode.window.createStatusBarItem(
		'gohawk.status',
		vscode.StatusBarAlignment.Right,
		90,
	);

	constructor() {
		this.item.name = 'gohawk';
		this.item.command = 'gohawk.showProblems';
		this.show({ state: 'checking', text: 'gohawk: starting…', findings: 0 });
	}

	show(status: Status): void {
		this.item.text = `${icons[status.state]} ${status.text}`;
		this.item.tooltip = status.tooltip ?? defaultTooltip;
		const background = backgrounds[status.state];
		this.item.backgroundColor =
			background === undefined ? undefined : new vscode.ThemeColor(background);
	}

	/** Shown only while a Go file is in front, so it does not follow the user into other languages. */
	setVisible(visible: boolean): void {
		if (visible) {
			this.item.show();
		} else {
			this.item.hide();
		}
	}

	dispose(): void {
		this.item.dispose();
	}
}
