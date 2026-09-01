import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { appendFile, mkdir, readFile, rename, writeFile } from 'node:fs/promises';
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { dirname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

import {
	type Annotation,
	annotationRevision,
	buildCodexCommand,
	buildPrompt,
	type CodexResult,
	isRecord,
	type PersistedState,
	parseCodexResult,
	parsePersistedState,
	parseSubmitPayload,
	parseThreadId,
	RESULT_SCHEMA,
	type ReviewJob,
	type SubmitPayload,
	selectNewRevisions,
	type WorkerStatus,
} from './supervisor-protocol.ts';

export type { ReviewJob, WorkerPhase, WorkerStatus } from './supervisor-protocol.ts';
export {
	annotationRevision,
	buildCodexCommand,
	parseCodexResult,
	parsePersistedState,
	parseSubmitPayload,
	parseThreadId,
	selectNewRevisions,
} from './supervisor-protocol.ts';

const TOOL_DIRECTORY = dirname(fileURLToPath(import.meta.url));
const SITE_DIRECTORY = resolve(TOOL_DIRECTORY, '../..');
const PROJECT_DIRECTORY = resolve(SITE_DIRECTORY, '..');
const RUNTIME_DIRECTORY = resolve(SITE_DIRECTORY, '.agentation-runtime');
const STATE_FILE = resolve(RUNTIME_DIRECTORY, 'worker-state.json');
const RESULT_SCHEMA_FILE = resolve(RUNTIME_DIRECTORY, 'result-schema.json');
const LOG_FILE = resolve(RUNTIME_DIRECTORY, 'worker.log');
const DEFAULT_PORT = 4848;
const MAX_BODY_BYTES = 1024 * 1024;
const AGENTATION_HTTP_URL = process.env.GOHAWK_AGENTATION_HTTP_URL ?? 'http://127.0.0.1:4747';

async function readState(): Promise<PersistedState> {
	try {
		const state = parsePersistedState(JSON.parse(await readFile(STATE_FILE, 'utf8')));
		if (!state) throw new Error('invalid state');
		return state;
	} catch {
		return { queue: [], completedRevisions: [] };
	}
}

async function writeState(state: PersistedState): Promise<void> {
	await mkdir(RUNTIME_DIRECTORY, { recursive: true });
	const temporary = `${STATE_FILE}.tmp`;
	await writeFile(temporary, `${JSON.stringify(state, null, 2)}\n`);
	await rename(temporary, STATE_FILE);
}

async function log(message: string): Promise<void> {
	await mkdir(RUNTIME_DIRECTORY, { recursive: true });
	await appendFile(LOG_FILE, `[${new Date().toISOString()}] ${message}\n`);
}

async function updateAnnotations(annotations: Annotation[], status: string): Promise<void> {
	await Promise.allSettled(
		annotations.map(async ({ id }) => {
			const response = await fetch(`${AGENTATION_HTTP_URL}/annotations/${encodeURIComponent(id)}`, {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ status }),
			});
			if (!response.ok) throw new Error(`annotation ${id}: ${response.status}`);
		}),
	);
}

async function readRequestBody(request: IncomingMessage): Promise<unknown> {
	return await new Promise((resolveBody, rejectBody) => {
		let size = 0;
		let body = '';
		request.setEncoding('utf8');
		request.on('data', (chunk: string) => {
			size += Buffer.byteLength(chunk);
			if (size > MAX_BODY_BYTES) {
				rejectBody(new Error('request body too large'));
				request.destroy();
				return;
			}
			body += chunk;
		});
		request.on('end', () => {
			try {
				resolveBody(JSON.parse(body || '{}'));
			} catch (error) {
				rejectBody(error);
			}
		});
		request.on('error', rejectBody);
	});
}

function sendJson(response: ServerResponse, status: number, body: unknown): void {
	response.writeHead(status, { 'Content-Type': 'application/json' });
	response.end(JSON.stringify(body));
}

class ReviewSupervisor {
	private state: PersistedState = { queue: [], completedRevisions: [] };
	private processing = false;
	private status: WorkerStatus = { phase: 'idle', queueLength: 0 };
	private readonly clients = new Set<ServerResponse>();

	async initialize(): Promise<void> {
		await mkdir(RUNTIME_DIRECTORY, { recursive: true });
		await writeFile(RESULT_SCHEMA_FILE, `${JSON.stringify(RESULT_SCHEMA, null, 2)}\n`);
		this.state = await readState();
		if (this.state.queue.length > 0) {
			this.setStatus({ phase: 'queued', queueLength: this.state.queue.length });
			queueMicrotask(() => void this.processQueue());
		}
	}

	addClient(response: ServerResponse): void {
		this.clients.add(response);
		response.write(`data: ${JSON.stringify(this.status)}\n\n`);
		response.on('close', () => this.clients.delete(response));
	}

	getStatus(): WorkerStatus {
		return this.status;
	}

	async enqueue(payload: SubmitPayload): Promise<{ job?: ReviewJob; duplicate: boolean }> {
		const selected = selectNewRevisions(payload, this.state.queue, this.state.completedRevisions);
		if (!selected) return { duplicate: true };

		const job: ReviewJob = {
			id: randomUUID(),
			createdAt: new Date().toISOString(),
			output: selected.output,
			annotations: selected.annotations,
		};
		this.state.queue.push(job);
		await writeState(this.state);
		this.setStatus({
			phase: 'queued',
			jobId: job.id,
			annotationCount: job.annotations.length,
			queueLength: this.state.queue.length,
		});
		queueMicrotask(() => void this.processQueue());
		return { job, duplicate: false };
	}

	private setStatus(status: WorkerStatus): void {
		this.status = status;
		const event = `data: ${JSON.stringify(status)}\n\n`;
		for (const client of this.clients) client.write(event);
	}

	private async processQueue(): Promise<void> {
		if (this.processing) return;
		this.processing = true;
		try {
			while (this.state.queue.length > 0) {
				const job = this.state.queue[0];
				if (!job) break;
				const completed = await this.runJob(job);
				this.state.queue.shift();
				if (completed) {
					const revisions = job.annotations.map(annotationRevision);
					this.state.completedRevisions = [
						...new Set([...this.state.completedRevisions, ...revisions]),
					];
				}
				await writeState(this.state);
			}
		} finally {
			this.processing = false;
		}
	}

	private async runJob(job: ReviewJob): Promise<boolean> {
		this.setStatus({
			phase: 'working',
			jobId: job.id,
			annotationCount: job.annotations.length,
			queueLength: this.state.queue.length - 1,
		});
		await updateAnnotations(job.annotations, 'acknowledged');

		const resultFile = resolve(RUNTIME_DIRECTORY, `result-${job.id}.json`);
		const command = buildCodexCommand({
			threadId: this.state.threadId,
			prompt: buildPrompt(job),
			projectDirectory: PROJECT_DIRECTORY,
			resultSchemaFile: RESULT_SCHEMA_FILE,
			resultFile,
		});
		await log(
			`starting ${this.state.threadId ? `thread ${this.state.threadId}` : 'new thread'} for job ${job.id}`,
		);

		const exitCode = await new Promise<number>((resolveExit) => {
			const child = spawn(command.command, command.args, {
				cwd: PROJECT_DIRECTORY,
				env: process.env,
				stdio: ['ignore', 'pipe', 'pipe'],
			});
			let stdoutBuffer = '';
			child.stdout.on('data', (chunk: Buffer) => {
				const text = chunk.toString();
				void appendFile(LOG_FILE, text);
				stdoutBuffer += text;
				const lines = stdoutBuffer.split('\n');
				stdoutBuffer = lines.pop() ?? '';
				for (const line of lines) {
					const threadId = parseThreadId(line);
					if (threadId && threadId !== this.state.threadId) {
						this.state.threadId = threadId;
						void writeState(this.state);
					}
				}
			});
			child.stderr.on('data', (chunk: Buffer) => void appendFile(LOG_FILE, chunk));
			child.on('error', (error) => {
				void log(`failed to start Codex: ${error.message}`);
				resolveExit(-1);
			});
			child.on('exit', (code) => resolveExit(code ?? -1));
		});

		let result: CodexResult | null = null;
		try {
			result = parseCodexResult(await readFile(resultFile, 'utf8'));
		} catch {
			result = null;
		}

		if (exitCode === 0 && result?.status === 'completed') {
			await updateAnnotations(job.annotations, 'resolved');
			this.setStatus({
				phase: 'done',
				jobId: job.id,
				annotationCount: job.annotations.length,
				queueLength: this.state.queue.length - 1,
				detail: result.summary,
			});
			await log(`completed job ${job.id}: ${result.summary}`);
			return true;
		}

		await updateAnnotations(job.annotations, 'pending');
		const detail = result?.summary ?? `Codex exited with code ${exitCode}`;
		this.setStatus({
			phase: 'failed',
			jobId: job.id,
			annotationCount: job.annotations.length,
			queueLength: this.state.queue.length - 1,
			detail,
		});
		await log(`failed job ${job.id}: ${detail}`);
		return false;
	}
}

export async function startSupervisor(): Promise<void> {
	const supervisor = new ReviewSupervisor();
	await supervisor.initialize();
	const port = Number.parseInt(process.env.GOHAWK_AGENTATION_REVIEW_PORT ?? `${DEFAULT_PORT}`, 10);

	const server = createServer(async (request, response) => {
		const url = new URL(request.url ?? '/', `http://${request.headers.host ?? '127.0.0.1'}`);
		if (request.method === 'GET' && url.pathname === '/health') {
			sendJson(response, 200, supervisor.getStatus());
			return;
		}
		if (request.method === 'GET' && url.pathname === '/events') {
			response.writeHead(200, {
				'Content-Type': 'text/event-stream',
				'Cache-Control': 'no-cache',
				Connection: 'keep-alive',
			});
			supervisor.addClient(response);
			return;
		}
		if (request.method === 'POST' && url.pathname === '/webhook') {
			try {
				const body = await readRequestBody(request);
				if (isRecord(body) && body.event !== 'submit') {
					sendJson(response, 202, { ignored: true });
					return;
				}
				const payload = parseSubmitPayload(body);
				if (!payload) {
					sendJson(response, 400, { error: 'invalid Agentation submit payload' });
					return;
				}
				const queued = await supervisor.enqueue(payload);
				sendJson(response, 202, {
					jobId: queued.job?.id,
					duplicate: queued.duplicate,
					annotationCount: queued.job?.annotations.length ?? 0,
				});
			} catch (error) {
				sendJson(response, 400, {
					error: error instanceof Error ? error.message : 'invalid request',
				});
			}
			return;
		}
		sendJson(response, 404, { error: 'not found' });
	});

	server.listen(port, '127.0.0.1', () => {
		console.log(`Agentation review supervisor listening on http://127.0.0.1:${port}`);
	});
}

const invokedAsScript = process.argv[1]
	? pathToFileURL(resolve(process.argv[1])).href === import.meta.url
	: false;
if (invokedAsScript) {
	startSupervisor().catch((error: unknown) => {
		console.error(error);
		process.exitCode = 1;
	});
}
