import { createHash } from 'node:crypto';

export type WorkerPhase = 'idle' | 'queued' | 'working' | 'done' | 'failed';

export type Annotation = {
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

export type AgentName = 'codex' | 'claude';

export function parseAgentName(value: unknown): AgentName | undefined {
	return value === 'codex' || value === 'claude' ? value : undefined;
}

export type PersistedState = {
	threadId?: string;
	agent?: AgentName;
	queue: ReviewJob[];
	completedRevisions: string[];
};

export type WorkerStatus = {
	phase: WorkerPhase;
	agent?: AgentName;
	jobId?: string;
	annotationCount?: number;
	queueLength: number;
	detail?: string;
};

export type CodexResult = {
	status: 'completed' | 'blocked';
	summary: string;
};

export type SubmitPayload = {
	event: 'submit';
	output: string;
	annotations: Annotation[];
};

export type CodexCommand = {
	command: string;
	args: string[];
};

export const RESULT_SCHEMA = {
	type: 'object',
	additionalProperties: false,
	properties: {
		status: { type: 'string', enum: ['completed', 'blocked'] },
		summary: { type: 'string' },
	},
	required: ['status', 'summary'],
};

export function isRecord(value: unknown): value is Record<string, unknown> {
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
		agent: parseAgentName(value.agent),
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
		if (!isRecord(event)) return undefined;
		// Codex streams a thread.started event; Claude Code streams a system/init
		// event whose session_id identifies the resumable conversation.
		let id: unknown;
		if (event.type === 'thread.started') {
			id = event.thread_id ?? event.threadId;
		} else if (event.type === 'system' && event.subtype === 'init') {
			id = event.session_id;
		} else {
			return undefined;
		}
		return typeof id === 'string' && id.length > 0 ? id : undefined;
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

export function buildPrompt(job: ReviewJob, resultFile?: string): string {
	const base = `You are the dedicated UI feedback worker for the gohawk repository.

Address the submitted Agentation feedback batch below. Work only inside this repository. Preserve unrelated working-tree changes. Do not commit or push. Do not modify the Agentation review tooling unless the feedback explicitly concerns that tooling. Inspect the current source rather than assuming a framework or file location. Make the requested changes, run focused checks appropriate to the files you touched, and verify the result as far as the local environment allows.

Return status "completed" only when every annotation in this batch has been addressed and the relevant checks pass. Return status "blocked" when clarification or unavailable prerequisites prevent completion, and explain the blocker in summary.

<submitted_feedback>
${job.output}
</submitted_feedback>`;
	if (!resultFile) return base;
	// Codex writes its structured last message to a file via a CLI flag; an
	// agent without that flag is told to write the same result itself.
	return `${base}

When you have finished, write your result as your final action to the file ${resultFile}: a single JSON object with exactly two fields, "status" (either "completed" or "blocked") and "summary" (a short explanation). Write only that JSON object to that file.`;
}

export function buildClaudeCommand(options: { threadId?: string; prompt: string }): CodexCommand {
	// Claude Code runs in the supervisor's working directory. Streaming JSON is
	// required so the system/init event exposes the session id parseThreadId
	// reads, and skipping permission prompts keeps the batch non-interactive.
	const base = [
		'--output-format',
		'stream-json',
		'--verbose',
		'--dangerously-skip-permissions',
		'-p',
		options.prompt,
	];
	if (options.threadId) {
		return { command: 'claude', args: ['--resume', options.threadId, ...base] };
	}
	return { command: 'claude', args: base };
}
