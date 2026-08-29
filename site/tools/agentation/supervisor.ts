import { spawn } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { appendFile, mkdir, readFile, rename, writeFile } from 'node:fs/promises';
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { dirname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

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

export type WorkerPhase = 'idle' | 'queued' | 'working' | 'done' | 'failed';

type Annotation = {
	id: string;
	comment?: string;
	element?: string;
	elementPath?: string;
	sessionId?: string;
	url?: string;
	[key: string]: unknown;
};

export type ReviewJob = {
	id: string;
	createdAt: string;
	output: string;
	annotations: Annotation[];
};

type PersistedState = {
	threadId?: string;
	queue: ReviewJob[];
	completedRevisions: string[];
};

export type WorkerStatus = {
	phase: WorkerPhase;
	jobId?: string;
	annotationCount?: number;
	queueLength: number;
	detail?: string;
};

type CodexResult = {
	status: 'completed' | 'blocked';
	summary: string;
};

type SubmitPayload = {
	event: 'submit';
	output: string;
	annotations: Annotation[];
};

type CodexCommand = {
	command: string;
	args: string[];
};

const RESULT_SCHEMA = {
	type: 'object',
	additionalProperties: false,
	properties: {
		status: { type: 'string', enum: ['completed', 'blocked'] },
		summary: { type: 'string' },
	},
	required: ['status', 'summary'],
};

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export function parseSubmitPayload(value: unknown): SubmitPayload | null {
	if (!isRecord(value) || value.event !== 'submit' || !Array.isArray(value.annotations)) {
		return null;
	}
	if (value.annotations.length === 0 || value.annotations.length > 100) return null;

	const annotations: Annotation[] = [];
	for (const candidate of value.annotations) {
		if (!isRecord(candidate) || typeof candidate.id !== 'string' || candidate.id.length === 0) {
			return null;
		}
		annotations.push(candidate as Annotation);
	}

	const output = typeof value.output === 'string' ? value.output.trim() : '';
	if (!output || output.length > 500_000) return null;
	return { event: 'submit', output, annotations };
}

export function annotationRevision({ id, comment }: Annotation): string {
	return createHash('sha256').update(JSON.stringify({ id, comment })).digest('hex');
}

function formatAnnotations(annotations: Annotation[]): string {
	const pages = [
		...new Set(annotations.map(({ url }) => url).filter((url) => typeof url === 'string')),
	];
	let output = '## Newly submitted page feedback\n\n';
	if (pages.length > 0) output += `**Page:** ${pages.join(', ')}\n\n`;
	for (const [index, annotation] of annotations.entries()) {
		output += `### ${index + 1}. ${annotation.element ?? 'Page element'}\n\n`;
		output += `${annotation.comment ?? ''}\n\n`;
		if (annotation.elementPath) output += `**Element path:** ${annotation.elementPath}\n\n`;
		if (typeof annotation.selectedText === 'string') {
			output += `**Selected text:** ${annotation.selectedText}\n\n`;
		}
		if (typeof annotation.nearbyText === 'string') {
			output += `**Nearby text:** ${annotation.nearbyText}\n\n`;
		}
		if (typeof annotation.sourceFile === 'string') {
			output += `**Source:** ${annotation.sourceFile}\n\n`;
		}
	}
	return output.trim();
}

export function selectNewRevisions(
	payload: SubmitPayload,
	queue: ReviewJob[],
	completedRevisions: Iterable<string>,
): SubmitPayload | null {
	const seen = new Set(completedRevisions);
	for (const job of queue) {
		for (const annotation of job.annotations) seen.add(annotationRevision(annotation));
	}

	const annotations = payload.annotations.filter((annotation) => {
		const revision = annotationRevision(annotation);
		if (seen.has(revision)) return false;
		seen.add(revision);
		return true;
	});
	if (annotations.length === 0) return null;
	return {
		...payload,
		output:
			annotations.length === payload.annotations.length
				? payload.output
				: formatAnnotations(annotations),
		annotations,
	};
}

export function parsePersistedState(value: unknown): PersistedState | null {
	if (!isRecord(value) || !Array.isArray(value.queue)) return null;
	return {
		threadId: typeof value.threadId === 'string' ? value.threadId : undefined,
		queue: value.queue as ReviewJob[],
		completedRevisions: Array.isArray(value.completedRevisions)
			? value.completedRevisions.filter(
					(revision): revision is string => typeof revision === 'string',
				)
			: [],
	};
}

export function parseThreadId(line: string): string | undefined {
	try {
		const event: unknown = JSON.parse(line);
		if (!isRecord(event) || event.type !== 'thread.started') return undefined;
		const threadId = event.thread_id ?? event.threadId;
		return typeof threadId === 'string' && threadId.length > 0 ? threadId : undefined;
	} catch {
		return undefined;
	}
}

export function parseCodexResult(value: string): CodexResult | null {
	try {
		const result: unknown = JSON.parse(value);
		if (!isRecord(result)) return null;
		if (result.status !== 'completed' && result.status !== 'blocked') return null;
		if (typeof result.summary !== 'string' || result.summary.length === 0) return null;
		return { status: result.status, summary: result.summary };
	} catch {
		return null;
	}
}

export function buildCodexCommand(options: {
	threadId?: string;
	prompt: string;
	projectDirectory: string;
	resultSchemaFile: string;
	resultFile: string;
}): CodexCommand {
	const resultOptions = [
		'--json',
		'--output-schema',
		options.resultSchemaFile,
		'--output-last-message',
		options.resultFile,
	];
	if (options.threadId) {
		return {
			command: 'codex',
			args: ['exec', 'resume', ...resultOptions, options.threadId, options.prompt],
		};
	}
	return {
		command: 'codex',
		args: [
			'--approve-for-me',
			'--cd',
			options.projectDirectory,
			'exec',
			...resultOptions,
			options.prompt,
		],
	};
}

function buildPrompt(job: ReviewJob): string {
	return `You are the dedicated UI feedback worker for the gohawk repository.

Address the submitted Agentation feedback batch below. Work only inside this repository. Preserve unrelated working-tree changes. Do not commit or push. Do not modify the Agentation review tooling unless the feedback explicitly concerns that tooling. Inspect the current source rather than assuming a framework or file location. Make the requested changes, run focused checks appropriate to the files you touched, and verify the result as far as the local environment allows.

Return status "completed" only when every annotation in this batch has been addressed and the relevant checks pass. Return status "blocked" when clarification or unavailable prerequisites prevent completion, and explain the blocker in summary.

<submitted_feedback>
${job.output}
</submitted_feedback>`;
}

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
