import assert from 'node:assert/strict';
import test from 'node:test';
import {
	annotationRevision,
	buildClaudeCommand,
	buildCodexCommand,
	parseAgentName,
	parseCodexResult,
	parsePersistedState,
	parseSubmitPayload,
	parseThreadId,
	selectNewRevisions,
} from './supervisor.ts';

const payload = {
	event: 'submit',
	output: '## Feedback\n\nFix the spacing.',
	annotations: [{ id: 'annotation-1', comment: 'Fix the spacing.' }],
} as const;

test('accepts complete Agentation submit payloads', () => {
	const parsed = parseSubmitPayload(payload);
	assert.ok(parsed);
	assert.equal(parsed.annotations.length, 1);
	assert.notEqual(
		annotationRevision(parsed.annotations[0]),
		annotationRevision({ ...parsed.annotations[0], comment: 'Edited comment.' }),
	);
});

test('filters queued and completed comment revisions', () => {
	const parsed = parseSubmitPayload({
		...payload,
		output: 'Original Agentation output containing both comments.',
		annotations: [
			...payload.annotations,
			{ id: 'annotation-2', comment: 'Increase the heading size.', element: 'h2' },
		],
	});
	assert.ok(parsed);
	const selected = selectNewRevisions(parsed, [], [annotationRevision(parsed.annotations[0])]);
	assert.ok(selected);
	assert.deepEqual(
		selected.annotations.map(({ id }) => id),
		['annotation-2'],
	);
	assert.match(selected.output, /Increase the heading size/);
	assert.doesNotMatch(selected.output, /Fix the spacing/);
});

test('treats an edited comment as a new revision', () => {
	const parsed = parseSubmitPayload(payload);
	assert.ok(parsed);
	const edited = parseSubmitPayload({
		...payload,
		output: '## Feedback\n\nFix all of the spacing.',
		annotations: [{ id: 'annotation-1', comment: 'Fix all of the spacing.' }],
	});
	assert.ok(edited);
	assert.ok(selectNewRevisions(edited, [], [annotationRevision(parsed.annotations[0])]));
});

test('rejects revisions that are already queued or completed', () => {
	const parsed = parseSubmitPayload(payload);
	assert.ok(parsed);
	const queued = {
		id: 'job-1',
		createdAt: '2026-08-29T00:00:00.000Z',
		output: parsed.output,
		annotations: parsed.annotations,
	};
	assert.equal(selectNewRevisions(parsed, [queued], []), null);
	assert.equal(selectNewRevisions(parsed, [], [annotationRevision(parsed.annotations[0])]), null);
});

test('preserves completed revisions across state reloads and migrates legacy state', () => {
	assert.deepEqual(parsePersistedState({ threadId: 'thread-1', queue: [] }), {
		threadId: 'thread-1',
		agent: undefined,
		queue: [],
		completedRevisions: [],
	});
	assert.deepEqual(
		parsePersistedState({ agent: 'claude', queue: [], completedRevisions: ['revision-1', 42] }),
		{
			threadId: undefined,
			agent: 'claude',
			queue: [],
			completedRevisions: ['revision-1'],
		},
	);
});

test('rejects annotation events and incomplete submissions', () => {
	assert.equal(parseSubmitPayload({ ...payload, event: 'annotation.add' }), null);
	assert.equal(parseSubmitPayload({ event: 'submit', output: '', annotations: [] }), null);
});

test('extracts Codex thread IDs and structured completion results', () => {
	assert.equal(
		parseThreadId(JSON.stringify({ type: 'thread.started', thread_id: 'thread-123' })),
		'thread-123',
	);
	assert.deepEqual(parseCodexResult('{"status":"completed","summary":"Fixed it."}'), {
		status: 'completed',
		summary: 'Fixed it.',
	});
	assert.equal(parseCodexResult('{"status":"completed"}'), null);
});

test('starts a sandboxed thread, then resumes the same thread', () => {
	const base = {
		prompt: 'Fix the feedback.',
		projectDirectory: '/repo',
		resultSchemaFile: '/runtime/schema.json',
		resultFile: '/runtime/result.json',
	};
	const initial = buildCodexCommand(base);
	assert.deepEqual(initial.args.slice(0, 4), ['--approve-for-me', '--cd', '/repo', 'exec']);
	assert.equal(initial.args.includes('--sandbox'), false);

	const resumed = buildCodexCommand({ ...base, threadId: 'thread-123' });
	assert.deepEqual(resumed.args.slice(0, 3), ['exec', 'resume', '--json']);
	assert.ok(resumed.args.includes('thread-123'));
});

test('extracts Claude Code session IDs from the init event', () => {
	assert.equal(
		parseThreadId(JSON.stringify({ type: 'system', subtype: 'init', session_id: 'session-abc' })),
		'session-abc',
	);
	// A non-init system event carries no resumable id.
	assert.equal(
		parseThreadId(JSON.stringify({ type: 'system', subtype: 'hook_started', session_id: 'x' })),
		undefined,
	);
});

test('builds a Claude Code command and resumes its session', () => {
	const initial = buildClaudeCommand({ prompt: 'Fix the feedback.' });
	assert.equal(initial.command, 'claude');
	assert.ok(initial.args.includes('--dangerously-skip-permissions'));
	assert.deepEqual(initial.args.slice(-2), ['-p', 'Fix the feedback.']);
	assert.equal(initial.args.includes('--resume'), false);

	const resumed = buildClaudeCommand({ prompt: 'More feedback.', threadId: 'session-abc' });
	assert.deepEqual(resumed.args.slice(0, 2), ['--resume', 'session-abc']);
});

test('parses the configurable agent name', () => {
	assert.equal(parseAgentName('codex'), 'codex');
	assert.equal(parseAgentName('claude'), 'claude');
	assert.equal(parseAgentName('gemini'), undefined);
	assert.equal(parseAgentName(undefined), undefined);
});
