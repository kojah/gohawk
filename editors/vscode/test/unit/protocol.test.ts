import assert from 'node:assert/strict';
import { test } from 'node:test';
import { parseFindingTarget, parseStatus } from '../../src/protocol.ts';

test('a well-formed status is accepted', () => {
	assert.deepEqual(parseStatus({ state: 'findings', text: 'gohawk: 1 finding', findings: 1 }), {
		state: 'findings',
		text: 'gohawk: 1 finding',
		findings: 1,
	});
	assert.equal(
		parseStatus({ state: 'skipped', text: 't', tooltip: 'why', findings: 0 })?.tooltip,
		'why',
	);
});

test('a status with an unknown state or missing fields is rejected', () => {
	assert.equal(parseStatus({ state: 'done', text: 't', findings: 0 }), undefined);
	assert.equal(parseStatus({ state: 'idle', findings: 0 }), undefined);
	assert.equal(parseStatus({ state: 'idle', text: 't', findings: '0' }), undefined);
	assert.equal(parseStatus(null), undefined);
	assert.equal(parseStatus([]), undefined);
});

test('a finding target needs a file, a one-based line and an analyzer', () => {
	const target = {
		uri: 'file:///m/header.go',
		line: 10,
		character: 11,
		analyzer: 'resourcelifetime',
		check: 'resourcelifetime/missing-release',
	};
	assert.deepEqual(parseFindingTarget(target), target);
	assert.equal(parseFindingTarget({ ...target, line: 0 }), undefined);
	assert.equal(parseFindingTarget({ ...target, line: 1.5 }), undefined);
	assert.equal(parseFindingTarget({ ...target, analyzer: '' }), undefined);
	assert.equal(parseFindingTarget({ ...target, uri: undefined }), undefined);
	assert.equal(parseFindingTarget('file:///m/header.go'), undefined);
});
