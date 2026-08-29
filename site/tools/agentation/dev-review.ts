import { type ChildProcess, spawn } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const TOOL_DIRECTORY = dirname(fileURLToPath(import.meta.url));
const SITE_DIRECTORY = resolve(TOOL_DIRECTORY, '../..');
const BIN_DIRECTORY = resolve(SITE_DIRECTORY, 'node_modules/.bin');
const children = new Set<ChildProcess>();
let stopping = false;

async function isHealthy(url: string): Promise<boolean> {
	try {
		return (await fetch(url)).ok;
	} catch {
		return false;
	}
}

async function waitForHealth(url: string, child: ChildProcess): Promise<void> {
	const deadline = Date.now() + 10_000;
	while (Date.now() < deadline) {
		if (child.exitCode !== null) throw new Error(`service exited with code ${child.exitCode}`);
		if (await isHealthy(url)) return;
		await new Promise((resolveWait) => setTimeout(resolveWait, 100));
	}
	throw new Error(`timed out waiting for ${url}`);
}

function start(command: string, args: string[]): ChildProcess {
	const child = spawn(command, args, { cwd: SITE_DIRECTORY, env: process.env, stdio: 'inherit' });
	children.add(child);
	child.on('exit', () => children.delete(child));
	return child;
}

function stop(exitCode = 0): void {
	if (stopping) return;
	stopping = true;
	for (const child of children) child.kill('SIGTERM');
	process.exitCode = exitCode;
}

async function ensureService(options: {
	healthUrl: string;
	command: string;
	args: string[];
	name: string;
}): Promise<void> {
	if (await isHealthy(options.healthUrl)) {
		console.log(`Reusing ${options.name} at ${options.healthUrl}`);
		return;
	}
	const child = start(options.command, options.args);
	await waitForHealth(options.healthUrl, child);
}

async function main(): Promise<void> {
	await ensureService({
		healthUrl: 'http://127.0.0.1:4747/health',
		command: resolve(BIN_DIRECTORY, 'agentation-mcp'),
		args: ['server', '--port', '4747'],
		name: 'Agentation sync server',
	});
	await ensureService({
		healthUrl: 'http://127.0.0.1:4848/health',
		command: process.execPath,
		args: [resolve(TOOL_DIRECTORY, 'supervisor.ts')],
		name: 'Agentation review supervisor',
	});

	const astroArguments = process.argv.slice(2);
	if (astroArguments[0] === '--') astroArguments.shift();
	const astro = start(resolve(BIN_DIRECTORY, 'astro'), ['dev', ...astroArguments]);
	astro.on('exit', (code) => {
		if (stopping) return;
		if (code === 0 && children.size > 0) {
			console.log('Astro is running in the background; review services remain attached here.');
			return;
		}
		stop(code ?? 1);
	});
}

process.on('SIGINT', () => stop(0));
process.on('SIGTERM', () => stop(0));

main().catch((error: unknown) => {
	console.error(error);
	stop(1);
});
