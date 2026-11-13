// Page helpers for driving VS Code in the browser. Scenarios use these and
// never touch selectors directly, so a VS Code markup change is fixed here.

import { readFile } from 'node:fs/promises';
import { join } from 'node:path';

const statusItem = '.statusbar-item[id="gohawk.gohawk.gohawk.status"]';
const stepItem = '.statusbar-item[id^="gohawk.gohawk"]:not([id="gohawk.gohawk.gohawk.status"])';

export function createEditor({ page, out, workspace, url }) {
	const pause = (ms) => page.waitForTimeout(ms);

	async function palette(command) {
		await page.keyboard.press('F1');
		await pause(300);
		await page.keyboard.insertText(command);
		await pause(600);
		await page.keyboard.press('Enter');
		await pause(700);
	}

	/** Closes anything a previous step left open, so keystrokes reach the workbench. */
	async function settle() {
		await page.keyboard.press('Escape');
		await page.keyboard.press('Escape');
		await pause(200);
	}

	async function openFile(relativePath, line) {
		await settle();
		await page.keyboard.press('Control+P');
		await pause(300);
		await page.keyboard.insertText(line ? `${relativePath}:${line}` : relativePath);
		await pause(900);
		await page.keyboard.press('Enter');
		await pause(1200);
	}

	async function textOf(selector) {
		const locator = page.locator(selector).first();
		return (await locator.count()) === 0 ? '' : (await locator.innerText()).trim();
	}

	const statusText = () => textOf(statusItem);
	const stepText = () => textOf(stepItem);

	/** Waits until the gohawk status matches, and returns how long that took. */
	async function waitForStatus(pattern, timeoutMs = 120_000) {
		const started = Date.now();
		let last = '';
		while (Date.now() - started < timeoutMs) {
			last = await statusText();
			if (pattern.test(last)) {
				return Date.now() - started;
			}
			await pause(50);
		}
		throw new Error(`gohawk status never matched ${pattern}; last was "${last}"`);
	}

	const squiggles = () =>
		page.locator('.editor-instance .squiggly-warning, .editor-instance .squiggly-info').count();
	const token = (text) =>
		page.locator('.editor-instance .view-lines span span', { hasText: text }).first();

	async function shot(entry, name) {
		const file = `${name}.png`;
		await page.screenshot({ path: join(out, file) });
		entry.screenshots.push(file);
	}

	/**
	 * Settings live in the browser for a web-served VS Code. This trusts the
	 * folder, turns auto-save off so "unsaved" means unsaved, and hides the
	 * panels that are not under test.
	 */
	async function prepare(theme, firstGoFile, editorFont) {
		const settings = {
			'security.workspace.trust.enabled': false,
			'workbench.colorTheme': theme,
			'files.autoSave': 'off',
			'workbench.startupEditor': 'none',
			'editor.minimap.enabled': false,
			'editor.fontSize': 14,
			'editor.lineHeight': 1.6,
			...(editorFont
				? {
						'editor.fontFamily': `'${editorFont}', monospace`,
						'terminal.integrated.fontFamily': editorFont,
					}
				: {}),
		};
		await pause(3000);
		await palette('Preferences: Open User Settings (JSON)');
		await page.locator('.editor-instance .view-lines').first().waitFor({ timeout: 30_000 });
		await page.locator('.editor-instance .view-lines').first().click();
		await page.keyboard.press('Control+A');
		await page.keyboard.insertText(JSON.stringify(settings, null, '\t'));
		await pause(300);
		await palette('File: Save');
		await pause(1000);
		await page.goto(url);
		await page.waitForSelector('.monaco-workbench', { timeout: 120_000 });
		await pause(4000);
		// Other extensions show a welcome tab the first time they see their
		// language. Trigger that now so it cannot take focus mid-scenario.
		await openFile(firstGoFile);
		await pause(8000);
		await palette('View: Close All Editors');
		await palette('View: Close Secondary Side Bar');
	}

	const eventLog = join(out, 'events.jsonl');
	const base = {
		eventLog,
		page,
		pause,
		palette,
		openFile,
		statusText,
		stepText,
		waitForStatus,
		squiggles,
		token,
		workspace,
		shot,
	};

	/** Returns the editor to a known state between scenarios. */
	async function reset() {
		await settle();
		await palette('File: Revert File');
		await palette('View: Close All Editors');
		await palette('Notifications: Clear All Notifications');
	}

	return {
		...base,
		prepare,
		reset,
		forScenario(entry) {
			return {
				...base,
				shot: (name) => shot(entry, name),
				check(name, passed, detail) {
					entry.checks.push({ name, passed: Boolean(passed), detail });
				},
				measure(name, value) {
					entry.measurements[name] = value;
				},
				readWorkspaceFile: (relativePath) => readFile(join(workspace, relativePath), 'utf8'),
				events: async () =>
					(await readFile(join(out, 'events.jsonl'), 'utf8').catch(() => ''))
						.split('\n')
						.filter((line) => line.trim() !== '')
						.map((line) => JSON.parse(line)),
			};
		},
	};
}
