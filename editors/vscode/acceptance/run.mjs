// UX acceptance harness. It installs the packaged extension into a real
// VS Code served to a headless browser, plays scripted scenarios, and writes
// a report: screenshots, the latency a user would feel, memory per check,
// and any sign that the server crashed or left processes behind.
//
// Usage: node acceptance/run.mjs --gohawk <binary> [--vsix <file>] [--out <dir>]
//        [--scenarios open,edit] [--workspace <dir>] [--with-go-extension]
//        [--theme "Default Dark Modern"] [--keep]

import { execFile, spawn } from 'node:child_process';
import { cp, mkdir, mkdtemp, readdir, readFile, rm, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { homedir, tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs, promisify } from 'node:util';
import { chromium } from 'playwright-core';
import { scenarios } from './scenarios.mjs';

const run = promisify(execFile);
const here = dirname(fileURLToPath(import.meta.url));
const packageRoot = resolve(here, '..');

const { values: options } = parseArgs({
	options: {
		gohawk: { type: 'string' },
		vsix: { type: 'string' },
		out: { type: 'string', default: join(packageRoot, 'out', 'acceptance') },
		scenarios: { type: 'string', default: Object.keys(scenarios).join(',') },
		workspace: { type: 'string', default: join(here, 'testdata', 'demo') },
		theme: { type: 'string', default: 'Default Dark Modern' },
		scale: { type: 'string', default: '2' },
		'with-go-extension': { type: 'boolean', default: false },
		'first-file': { type: 'string', default: 'clean/clean.go' },
		keep: { type: 'boolean', default: false },
	},
});

if (!options.gohawk) {
	console.error('acceptance: --gohawk <path to a gohawk binary with the lsp command> is required');
	process.exit(2);
}

const sleep = (ms) => new Promise((done) => setTimeout(done, ms));

async function freePort() {
	return new Promise((done, fail) => {
		const server = createServer();
		server.once('error', fail);
		server.listen(0, '127.0.0.1', () => {
			const { port } = server.address();
			server.close(() => done(port));
		});
	});
}

/** The VS Code command-line launcher, downloaded once per machine. */
async function vscodeCLI() {
	const cache = join(packageRoot, '.vscode-test', 'cli');
	const binary = join(cache, process.platform === 'win32' ? 'code.exe' : 'code');
	try {
		await run(binary, ['--version']);
		return binary;
	} catch {
		// Not downloaded yet, or not runnable: fetch it below.
	}
	const targets = { linux: 'cli-linux', darwin: 'cli-darwin', win32: 'cli-win32' };
	const architectures = { x64: 'x64', arm64: 'arm64' };
	const target = `${targets[process.platform]}-${architectures[process.arch]}`;
	await mkdir(cache, { recursive: true });
	const response = await fetch(`https://update.code.visualstudio.com/latest/${target}/stable`, {
		signal: AbortSignal.timeout(180_000),
	});
	if (!response.ok) {
		throw new Error(`download VS Code CLI (${target}): HTTP ${response.status}`);
	}
	const archive = join(cache, 'cli-archive');
	await writeFile(archive, Buffer.from(await response.arrayBuffer()));
	await run('tar', ['-xf', archive, '-C', cache]);
	await rm(archive);
	return binary;
}

/** A Chromium the installed Playwright can drive. */
async function chromiumPath() {
	if (process.env.ACCEPTANCE_CHROMIUM) {
		return process.env.ACCEPTANCE_CHROMIUM;
	}
	const cache = join(homedir(), '.cache', 'ms-playwright');
	const shells = (await readdir(cache).catch(() => []))
		.filter((name) => name.startsWith('chromium_headless_shell'))
		.sort();
	const newest = shells.at(-1);
	if (newest) {
		const inner = (await readdir(join(cache, newest))).find((name) => name.startsWith('chrome'));
		return join(cache, newest, inner, 'chrome-headless-shell');
	}
	return chromium.executablePath();
}

async function waitForServer(port) {
	for (let attempt = 0; attempt < 120; attempt++) {
		try {
			const response = await fetch(`http://127.0.0.1:${port}/`, {
				signal: AbortSignal.timeout(2000),
			});
			if (response.status < 500) {
				return;
			}
		} catch {
			// Not listening yet.
		}
		await sleep(500);
	}
	throw new Error('the VS Code server did not start');
}

async function findServerBinary(cliData) {
	const root = join(cliData, 'serve-web');
	for (let attempt = 0; attempt < 240; attempt++) {
		for (const commit of await readdir(root).catch(() => [])) {
			const candidate = join(
				root,
				commit,
				'bin',
				process.platform === 'win32' ? 'code-server.cmd' : 'code-server',
			);
			try {
				await run(candidate, ['--version']);
				return candidate;
			} catch {
				// Still downloading or unpacking.
			}
		}
		await sleep(500);
	}
	throw new Error('the VS Code server was not downloaded');
}

/** Processes this run started that are still alive: go vet or gohawk tools. */
async function strayProcesses(marker) {
	if (process.platform === 'win32') {
		return [];
	}
	const { stdout } = await run('ps', ['-eo', 'pid=,args=']);
	return stdout
		.split('\n')
		.filter((line) => line.includes(marker) && /\bvet\b|-vettool|\.cfg\b/.test(line))
		.map((line) => line.trim().slice(0, 160));
}

function readEvents(text) {
	return text
		.split('\n')
		.filter((line) => line.trim() !== '')
		.map((line) => JSON.parse(line));
}

/** For every edit, the time until findings for that file were next published. */
function editLatencies(events) {
	const latencies = [];
	for (const [index, event] of events.entries()) {
		if (event.event !== 'document' || event.kind !== 'change') {
			continue;
		}
		const later = events.slice(index + 1);
		const superseded = later.findIndex(
			(next) => next.event === 'document' && next.kind === 'change' && next.path === event.path,
		);
		const answered = later.findIndex(
			(next) => next.event === 'published' && next.path === event.path,
		);
		if (answered >= 0 && (superseded < 0 || answered < superseded)) {
			latencies.push(Date.parse(later[answered].time) - Date.parse(event.time));
		}
	}
	return latencies;
}

function summarize(numbers) {
	if (numbers.length === 0) {
		return undefined;
	}
	const sorted = [...numbers].sort((a, b) => a - b);
	const at = (fraction) =>
		sorted[Math.min(sorted.length - 1, Math.floor(fraction * sorted.length))];
	return {
		count: sorted.length,
		min: sorted[0],
		median: at(0.5),
		p90: at(0.9),
		max: sorted.at(-1),
	};
}

async function main() {
	const gohawk = resolve(options.gohawk);
	const out = resolve(options.out);
	await rm(out, { recursive: true, force: true });
	await mkdir(out, { recursive: true });
	const sandbox = await mkdtemp(join(tmpdir(), 'gohawk-acceptance-'));
	const workspace = join(sandbox, 'workspace');
	const serverData = join(sandbox, 'server');
	const cliData = join(sandbox, 'cli');
	const eventLog = join(out, 'events.jsonl');
	await cp(resolve(options.workspace), workspace, { recursive: true });

	let vsix = options.vsix && resolve(options.vsix);
	if (!vsix) {
		await run('pnpm', ['run', 'package'], { cwd: packageRoot });
		const built = (await readdir(packageRoot))
			.filter((name) => name.endsWith('.vsix'))
			.sort()
			.at(-1);
		vsix = join(packageRoot, built);
	}

	await mkdir(join(serverData, 'data', 'Machine'), { recursive: true });
	await writeFile(
		join(serverData, 'data', 'Machine', 'settings.json'),
		JSON.stringify({
			'gohawk.path': gohawk,
			'gohawk.eventLog': eventLog,
			'go.toolsManagement.autoUpdate': false,
		}),
	);

	const port = await freePort();
	const cli = await vscodeCLI();
	const serverLog = [];
	const server = spawn(
		cli,
		[
			'serve-web',
			'--without-connection-token',
			'--accept-server-license-terms',
			'--host',
			'127.0.0.1',
			'--port',
			String(port),
			'--server-data-dir',
			serverData,
			'--cli-data-dir',
			cliData,
		],
		{ env: { ...process.env, GOWORK: 'off' }, detached: process.platform !== 'win32' },
	);
	server.stdout.on('data', (chunk) => serverLog.push(chunk.toString()));
	server.stderr.on('data', (chunk) => serverLog.push(chunk.toString()));
	const stopServer = () => {
		try {
			if (process.platform === 'win32') {
				server.kill();
			} else {
				process.kill(-server.pid, 'SIGTERM');
			}
		} catch {
			// Already gone.
		}
	};

	const report = { startedAt: new Date().toISOString(), gohawk, vsix, scenarios: [], problems: [] };
	let browser;
	try {
		await waitForServer(port);
		// The first page load makes the launcher download the server itself.
		browser = await chromium.launchPersistentContext(join(sandbox, 'browser'), {
			executablePath: await chromiumPath(),
			viewport: { width: 1440, height: 900 },
			deviceScaleFactor: Number(options.scale),
		});
		const page = browser.pages()[0] ?? (await browser.newPage());
		const url = `http://127.0.0.1:${port}/?folder=${encodeURIComponent(workspace)}`;
		await page.goto(url);
		await page.waitForSelector('.monaco-workbench', { timeout: 180_000 });

		const serverBinary = await findServerBinary(cliData);
		const extensions = [vsix, ...(options['with-go-extension'] ? ['golang.go'] : [])];
		for (const extension of extensions) {
			await run(serverBinary, [
				'--server-data-dir',
				serverData,
				'--extensions-dir',
				join(serverData, 'extensions'),
				'--install-extension',
				extension,
			]);
		}

		const editor = await import('./editor.mjs').then((module) =>
			module.createEditor({ page, out, workspace, url }),
		);
		await editor.prepare(options.theme, options['first-file']);

		for (const name of options.scenarios.split(',').filter(Boolean)) {
			const scenario = scenarios[name];
			if (!scenario) {
				throw new Error(`unknown scenario ${name}; known: ${Object.keys(scenarios).join(', ')}`);
			}
			const entry = { name, checks: [], screenshots: [], measurements: {} };
			report.scenarios.push(entry);
			const started = Date.now();
			try {
				await editor.reset();
				await scenario(editor.forScenario(entry));
				entry.outcome = entry.checks.every((check) => check.passed) ? 'passed' : 'failed';
			} catch (error) {
				entry.outcome = 'error';
				entry.error = String(error?.stack ?? error);
				await editor.shot(entry, `${name}-error`);
			}
			entry.durationMs = Date.now() - started;
			console.log(`${entry.outcome.padEnd(7)} ${name} (${entry.durationMs} ms)`);
		}

		const status = await editor.statusText();
		if (/server stopped|failed to start|not installed/.test(status)) {
			report.problems.push(`the status bar ended as "${status}"`);
		}
	} finally {
		await browser?.close().catch(() => undefined);
		await sleep(1500);
		const strays = await strayProcesses(sandbox).catch(() => []);
		if (strays.length > 0) {
			report.problems.push(`processes left running after the session: ${strays.join(' ; ')}`);
		}
		stopServer();
	}

	const events = readEvents(await readFile(eventLog, 'utf8').catch(() => ''));
	const finished = events.filter((event) => event.event === 'run-finished');
	report.server = {
		events: events.length,
		runs: finished.length,
		outcomes: Object.fromEntries(
			Map.groupBy(finished, (event) => event.outcome)
				.entries()
				.map(([key, group]) => [key, group.length]),
		),
		editToFindingsMs: summarize(editLatencies(events)),
		packageRunMs: summarize(
			finished
				.filter((event) => event.kind === 'package' && event.outcome === 'completed')
				.map((event) => event.elapsedMs ?? 0),
		),
		serverMemoryMiB: summarize(
			finished
				.filter((event) => event.serverBytes > 0)
				.map((event) => Math.round(event.serverBytes / 2 ** 20)),
		),
		serverGoroutines: summarize(
			finished.filter((event) => event.goroutines > 0).map((event) => event.goroutines),
		),
		peakMemoryMiB: summarize(
			finished
				.filter((event) => event.peakBytes > 0)
				.map((event) => Math.round(event.peakBytes / 2 ** 20)),
		),
	};
	const hostErrors = serverLog
		.join('')
		.split('\n')
		.filter((line) => /gohawk/i.test(line) && /error|crash|exited|SIGSEGV|panic/i.test(line));
	if (hostErrors.length > 0) {
		report.problems.push(
			`extension host log mentions gohawk failures: ${hostErrors.slice(0, 5).join(' | ')}`,
		);
	}
	await writeFile(join(out, 'server.log'), serverLog.join(''));
	await writeFile(join(out, 'report.json'), JSON.stringify(report, null, '\t'));
	await writeFile(join(out, 'report.md'), renderReport(report));
	if (!options.keep) {
		await rm(sandbox, { recursive: true, force: true });
	} else {
		console.log(`kept sandbox at ${sandbox}`);
	}

	const failed = report.scenarios.filter((entry) => entry.outcome !== 'passed');
	console.log(`report: ${join(out, 'report.md')}`);
	if (failed.length > 0 || report.problems.length > 0) {
		for (const problem of report.problems) {
			console.error(`problem: ${problem}`);
		}
		process.exitCode = 1;
	}
}

function renderReport(report) {
	const lines = [
		'# gohawk extension acceptance report',
		'',
		`Run at ${report.startedAt} with \`${report.gohawk}\`.`,
		'',
	];
	const number = (stats) =>
		stats
			? `median ${stats.median}, p90 ${stats.p90}, max ${stats.max} (n=${stats.count})`
			: 'not measured';
	lines.push('## Server measurements', '');
	lines.push(`- Runs: ${report.server.runs} (${JSON.stringify(report.server.outcomes)})`);
	lines.push(`- Edit to published findings, ms: ${number(report.server.editToFindingsMs)}`);
	lines.push(`- Package check, ms: ${number(report.server.packageRunMs)}`);
	lines.push(`- Peak memory per check, MiB: ${number(report.server.peakMemoryMiB)}`);
	lines.push(`- Server process memory, MiB: ${number(report.server.serverMemoryMiB)}`);
	lines.push(`- Server goroutines: ${number(report.server.serverGoroutines)}`);
	lines.push(
		'',
		'## Problems',
		'',
		...(report.problems.length > 0
			? report.problems.map((problem) => `- ${problem}`)
			: ['None detected.']),
		'',
	);
	for (const entry of report.scenarios) {
		lines.push(`## ${entry.name}: ${entry.outcome}`, '');
		for (const check of entry.checks) {
			lines.push(
				`- ${check.passed ? 'pass' : 'FAIL'}: ${check.name}${check.detail ? ` (${check.detail})` : ''}`,
			);
		}
		for (const [name, value] of Object.entries(entry.measurements)) {
			lines.push(`- measured ${name}: ${value}`);
		}
		if (entry.error) {
			lines.push('', '```', entry.error, '```');
		}
		lines.push('', ...entry.screenshots.map((file) => `![${file}](${file})`), '');
	}
	return lines.join('\n');
}

await main();
