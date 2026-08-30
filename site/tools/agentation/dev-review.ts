import { type ChildProcess, spawn } from 'node:child_process';
import { once } from 'node:events';
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

function start(command: string, args: string[], env = process.env): ChildProcess {
	const child = spawn(command, args, { cwd: SITE_DIRECTORY, env, stdio: 'inherit' });
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

async function run(command: string, args: string[]): Promise<void> {
	const child = spawn(command, args, { cwd: SITE_DIRECTORY, env: process.env, stdio: 'inherit' });
	const [code] = (await once(child, 'exit')) as [number | null];
	if (code !== 0) throw new Error(`${command} exited with code ${code}`);
}

function startAstro(astroArguments: string[]): void {
	const child = start(resolve(BIN_DIRECTORY, 'astro'), ['dev', ...astroArguments], {
		...process.env,
		// Astro otherwise auto-detaches in detected AI environments. Keeping it in
		// the foreground lets this supervisor restart the exact process it owns.
		ASTRO_DEV_BACKGROUND: '1',
	});
	child.on('exit', (code) => {
		if (!stopping) stop(code ?? 1);
	});
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
	// Replace a detached server left by an earlier agent-aware Astro invocation.
	await run(resolve(BIN_DIRECTORY, 'astro'), ['dev', 'stop']);
	startAstro(astroArguments);
}

process.on('SIGINT', () => stop(0));
process.on('SIGTERM', () => stop(0));

if (resolve(process.argv[1] ?? '') === fileURLToPath(import.meta.url)) {
	main().catch((error: unknown) => {
		console.error(error);
		stop(1);
	});
}
