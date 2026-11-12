// Scripted sessions. Each one checks something a user would notice and
// records what they would wait for. Scenarios run in order in one editor.

import { execFile } from 'node:child_process';
import { promisify } from 'node:util';

const run = promisify(execFile);
const oneFinding = /gohawk: 1 finding/;
// A restarted server re-checks every file the editor still holds open.
const anyFindings = /gohawk: \d+ findings?/;

export const scenarios = {
	/** A leaking function shows its finding, evidence and help. */
	async open(editor) {
		await editor.openFile('leak/header.go', 10);
		editor.measure('open to first findings, ms', await editor.waitForStatus(oneFinding));
		editor.check('the finding is underlined in the editor', (await editor.squiggles()) > 0);

		await editor.token('Open').hover();
		const hover = editor.page.locator('.monaco-hover-content').first();
		await hover.waitFor({ timeout: 15_000 });
		await editor.pause(1200);
		const hoverText = await hover.innerText();
		editor.check(
			'hover names the finding',
			hoverText.includes('not released on every return path'),
		);
		editor.check(
			'hover lists the evidence path',
			hoverText.includes('when this is true') &&
				hoverText.includes('returns here without releasing'),
		);
		editor.check('hover gives the usual fix', hoverText.includes('gohawk help:'));
		await editor.shot('open-hover');
		await editor.page.mouse.move(300, 700);

		await editor.palette('View: Focus Problems');
		await editor.pause(1000);
		const problems = await editor.page.locator('.markers-panel').innerText();
		editor.check(
			'Problems shows the check ID',
			problems.includes('resourcelifetime/missing-release'),
		);
		editor.check('Problems shows the evidence steps', problems.includes('when this is true'));
		await editor.shot('open-problems');
	},

	/** The lightbulb offers navigation and explanation, never an edit. */
	async actions(editor) {
		await editor.openFile('leak/header.go', 10);
		await editor.waitForStatus(oneFinding);
		await editor.token('Open').click();
		await editor.page.keyboard.press('Control+.');
		const menu = editor.page.locator('.action-widget').first();
		await menu.waitFor({ timeout: 15_000 });
		const offered = await menu.innerText();
		editor.check(
			'lightbulb offers path stepping',
			offered.includes('gohawk: Step through the reported path'),
		);
		editor.check(
			'lightbulb offers the explanation',
			offered.includes('gohawk: Explain this finding'),
		);
		editor.check('lightbulb offers documentation', offered.includes('gohawk: Open documentation'));
		await editor.shot('actions-lightbulb');
		await editor.page.keyboard.press('Escape');

		await editor.palette('gohawk: Step Through Path');
		await editor.pause(800);
		editor.check('stepping starts at step 1 of 3', (await editor.stepText()).includes('path 1/3'));
		await editor.palette('gohawk: Next Path Step');
		await editor.pause(600);
		editor.check(
			'next moves to the branch condition',
			(await editor.stepText()).includes('path 2/3 · when this is true'),
		);
		await editor.shot('actions-step');
		await editor.palette('gohawk: Exit Path Stepping');
		editor.check('exit ends stepping', (await editor.stepText()) === '');

		await editor.palette('gohawk: Explain Finding');
		const explained = editor.page
			.locator('.editor-instance .view-lines', { hasText: 'gohawk dump trace' })
			.first();
		await explained.waitFor({ timeout: 60_000 });
		editor.check(
			'explain shows the tracer output for this finding',
			(await explained.innerText()).includes('resourcelifetime'),
		);
		await editor.shot('actions-explain');
		await editor.palette('View: Close Editor');
	},

	/** An unsaved fix clears the finding; a broken build never looks clean. */
	async edit(editor) {
		await editor.openFile('leak/header.go', 15);
		await editor.waitForStatus(oneFinding);
		await editor.page.keyboard.press('Home');
		await editor.page.keyboard.insertText('\tdefer f.Close()\n');
		editor.measure(
			'typed fix to cleared finding, ms',
			await editor.waitForStatus(/gohawk: no findings/),
		);
		const onDisk = await editor.readWorkspaceFile('leak/header.go');
		editor.check('the fix was analyzed without being saved', !onDisk.includes('defer f.Close()'));
		editor.check('the underline is gone', (await editor.squiggles()) === 0);
		await editor.shot('edit-unsaved-fix');

		await editor.page.keyboard.insertText('\tthis is not go\n');
		await editor.waitForStatus(/build failed/);
		editor.check(
			'a broken build is reported as such',
			!/no findings/.test(await editor.statusText()),
		);
		await editor.shot('edit-broken-build');

		await editor.palette('File: Revert File');
		editor.measure('revert to finding restored, ms', await editor.waitForStatus(oneFinding));
	},

	/** A lock-order cycle reports both orders as related locations. */
	async locks(editor) {
		await editor.openFile('locks/locks.go', 19);
		await editor.waitForStatus(anyFindings);
		await editor.page
			.locator('.editor-instance .squiggly-warning')
			.first()
			.waitFor({ timeout: 60_000 });
		await editor.palette('View: Focus Problems');
		await editor.pause(1000);
		const problems = await editor.page.locator('.markers-panel').innerText();
		editor.check(
			'the cycle is reported',
			problems.includes('contradictory lock order: accounts and ledger'),
		);
		editor.check(
			'both lock orders are shown',
			problems.includes('is locked while') && problems.split('is locked while').length >= 3,
		);
		await editor.shot('locks-problems');
	},

	/** Correct code and library files produce nothing. */
	async quiet(editor) {
		await editor.openFile('clean/clean.go', 8);
		await editor.pause(6000);
		editor.check('a correct file has no underline', (await editor.squiggles()) === 0);
		await editor.token('Open').click();
		await editor.page.keyboard.press('F12');
		await editor.pause(6000);
		const runs = (await editor.events())
			.filter((event) => event.event === 'run-started')
			.map((event) => event.label);
		editor.check(
			'a standard library file opened by go-to-definition is not analyzed',
			!runs.includes('os'),
			`runs: ${[...new Set(runs)].join(', ')}`,
		);
		await editor.shot('quiet-library-file');
		await editor.palette('View: Close Editor');
	},

	/** Killing the server is noticed and recovered from, with findings restored. */
	async crash(editor) {
		await editor.openFile('leak/header.go', 10);
		await editor.waitForStatus(anyFindings);
		const { stdout } = await run('ps', ['-eo', 'pid=,args=']);
		const servers = stdout
			.split('\n')
			.map((line) => line.trim())
			.filter(
				(line) =>
					/gohawk(\.exe)? lsp/.test(line) &&
					line.includes(editor.workspace.replace(/workspace$/, '')) === false,
			)
			.filter((line) => line.includes('-event-log'));
		const mine = servers.find((line) => line.includes('events.jsonl'));
		editor.check('the server process is running', mine !== undefined);
		if (mine === undefined) {
			return;
		}
		process.kill(Number(mine.split(/\s+/)[0]), 'SIGKILL');
		const before = (await editor.events()).length;
		const killedAt = Date.now();
		// The editor restarts the server and sends it the open files again.
		while ((await editor.events()).length === before) {
			if (Date.now() - killedAt > 60_000) {
				throw new Error('no server answered within a minute of the kill');
			}
			await editor.pause(100);
		}
		await editor.waitForStatus(anyFindings, 60_000);
		editor.measure('kill to findings restored, ms', Date.now() - killedAt);
		editor.check('a new server answered after the kill', (await editor.events()).length > before);
		await editor.shot('crash-recovered');
		await editor.palette('File: Revert File');
	},
};
