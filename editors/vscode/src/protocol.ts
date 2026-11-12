// Messages that cross the process boundary from the gohawk server. They are
// parsed here once; the rest of the extension only sees validated values.

const statusStates = [
	'idle',
	'findings',
	'checking',
	'stale',
	'stopped',
	'error',
	'skipped',
] as const;

export type StatusState = (typeof statusStates)[number];

/** The payload of the server's gohawk/status notification. */
export type Status = {
	state: StatusState;
	text: string;
	tooltip?: string;
	findings: number;
};

/** Names one finding: where it is reported and which check reported it. */
export type FindingTarget = {
	uri: string;
	/** One-based, as gohawk prints it. */
	line: number;
	/** Zero-based UTF-16 column, as the protocol counts it. */
	character: number;
	analyzer: string;
	check: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isStatusState(value: unknown): value is StatusState {
	return statusStates.some((state) => state === value);
}

export function parseStatus(value: unknown): Status | undefined {
	if (!isRecord(value)) {
		return undefined;
	}
	const { state, text, tooltip, findings } = value;
	if (!isStatusState(state) || typeof text !== 'string' || typeof findings !== 'number') {
		return undefined;
	}
	return typeof tooltip === 'string' && tooltip !== ''
		? { state, text, tooltip, findings }
		: { state, text, findings };
}

export function parseFindingTarget(value: unknown): FindingTarget | undefined {
	if (!isRecord(value)) {
		return undefined;
	}
	const { uri, line, character, analyzer, check } = value;
	if (
		typeof uri !== 'string' ||
		typeof analyzer !== 'string' ||
		typeof check !== 'string' ||
		typeof line !== 'number' ||
		typeof character !== 'number' ||
		!Number.isInteger(line) ||
		!Number.isInteger(character) ||
		line < 1 ||
		character < 0 ||
		uri === '' ||
		analyzer === ''
	) {
		return undefined;
	}
	return { uri, line, character, analyzer, check };
}
