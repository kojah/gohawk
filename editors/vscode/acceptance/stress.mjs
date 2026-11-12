// Measures the server on a real package: how long a check takes after an
// edit, how much memory it uses, and what the importer run costs. It speaks
// the protocol to `gohawk lsp` directly, with no editor, so the numbers are
// the server's own.
//
// Usage: node acceptance/stress.mjs --gohawk <binary> --workspace <dir> --file <path>
//        [--edits 3] [--scope packageThenImporters] [--timeout 60]
//        [--memory 2GiB] [--parallel 2] [--wait 180] [--json]
//
// The workspace is never modified: edits exist only as unsaved buffers.

import { spawn } from 'node:child_process';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { parseArgs } from 'node:util';

const { values: options } = parseArgs({
	options: {
		gohawk: { type: 'string' },
		workspace: { type: 'string' },
		file: { type: 'string' },
		edits: { type: 'string', default: '3' },
		scope: { type: 'string', default: 'packageThenImporters' },
		timeout: { type: 'string', default: '60' },
		memory: { type: 'string', default: '2GiB' },
		parallel: { type: 'string', default: '2' },
		wait: { type: 'string', default: '180' },
		json: { type: 'boolean', default: false },
	},
});

if (!options.gohawk || !options.workspace || !options.file) {
	console.error('stress: --gohawk, --workspace and --file are required');
	process.exit(2);
}

const workspace = resolve(options.workspace);
const file = resolve(workspace, options.file);
const scratch = await mkdtemp(join(tmpdir(), 'gohawk-stress-'));
const eventLog = join(scratch, 'events.jsonl');
const sleep = (ms) => new Promise((done) => setTimeout(done, ms));

const server = spawn(resolve(options.gohawk), ['lsp', '-event-log', eventLog], {
	stdio: ['pipe', 'pipe', 'inherit'],
});
// Replies are not needed: everything measured comes from the event log.
server.stdout.resume();

function send(message) {
	const body = JSON.stringify({ jsonrpc: '2.0', ...message });
	server.stdin.write(`Content-Length: ${Buffer.byteLength(body)}\r\n\r\n${body}`);
}

async function events() {
	const text = await readFile(eventLog, 'utf8').catch(() => '');
	return text
		.split('\n')
		.filter((line) => line.trim() !== '')
		.map((line) => JSON.parse(line));
}

/** Waits for the first run of a kind to finish after position `from` in the log. */
async function finished(kind, from, seconds) {
	const deadline = Date.now() + seconds * 1000;
	while (Date.now() < deadline) {
		const all = await events();
		const found = all
			.slice(from)
			.find((event) => event.event === 'run-finished' && event.kind === kind);
		if (found) {
			return found;
		}
		await sleep(100);
	}
	return undefined;
}

const mebibytes = (bytes) => (bytes > 0 ? Math.round(bytes / 2 ** 20) : undefined);
const describe = (run) =>
	run
		? {
				outcome: run.outcome,
				seconds: Number((run.elapsedMs / 1000).toFixed(1)),
				peakMiB: mebibytes(run.peakBytes),
				findings: run.findings,
			}
		: { outcome: 'not finished within the wait' };

const uri = pathToFileURL(file).toString();
const original = await readFile(file, 'utf8');
const waitSeconds = Number(options.wait);
const rounds = [];
// go vet caches results by content, so each run's edits must be text no
// earlier run produced; otherwise an edit measures a cache hit.
const runStamp = Date.now();

send({
	id: 1,
	method: 'initialize',
	params: {
		rootUri: pathToFileURL(workspace).toString(),
		initializationOptions: {
			settings: {
				debounce: 100,
				scope: options.scope,
				timeout: Number(options.timeout),
				memoryLimit: options.memory,
				maxParallel: Number(options.parallel),
			},
		},
	},
});
send({ method: 'initialized', params: {} });

async function round(label, change) {
	const from = (await events()).length;
	const started = Date.now();
	change();
	const packageRun = await finished('package', from, waitSeconds);
	const result = { round: label, package: describe(packageRun) };
	if (packageRun) {
		result.package.editToDoneSeconds = Number(
			((Date.parse(packageRun.time) - started) / 1000).toFixed(1),
		);
	}
	if (options.scope === 'packageThenImporters' && packageRun?.outcome === 'completed') {
		result.importers = describe(await finished('importers', from, waitSeconds));
	}
	rounds.push(result);
	if (!options.json) {
		console.log(JSON.stringify(result));
	}
}

await round('open', () =>
	send({
		method: 'textDocument/didOpen',
		params: { textDocument: { uri, languageId: 'go', version: 1, text: original } },
	}),
);
for (let edit = 1; edit <= Number(options.edits); edit++) {
	await round(`edit ${edit}`, () =>
		send({
			method: 'textDocument/didChange',
			params: {
				textDocument: { uri, version: edit + 1 },
				contentChanges: [{ text: `${original}\n// stress edit ${edit} of run ${runStamp}\n` }],
			},
		}),
	);
}

const all = await events();
const runs = all.filter((event) => event.event === 'run-finished');
const summary = {
	file: options.file,
	rounds,
	server: {
		maxMemoryMiB: Math.max(0, ...runs.map((run) => mebibytes(run.serverBytes) ?? 0)),
		maxGoroutines: Math.max(0, ...runs.map((run) => run.goroutines ?? 0)),
	},
};

send({ id: 2, method: 'shutdown' });
send({ method: 'exit' });
await Promise.race([new Promise((done) => server.once('exit', done)), sleep(10_000)]);
server.kill();
await rm(scratch, { recursive: true, force: true });

if (options.json) {
	console.log(JSON.stringify(summary, null, '\t'));
} else {
	console.log(JSON.stringify({ server: summary.server }));
}
