import assert from 'node:assert/strict';
import test from 'node:test';
import { shouldRestartAstro } from './dev-review.ts';

test('restarts Astro for external documentation changes', () => {
	assert.equal(shouldRestartAstro('docs', 'configuration.md'), true);
	assert.equal(shouldRestartAstro('docs', 'analyzers/lockorder.mdx'), true);
	assert.equal(shouldRestartAstro('docs', 'notes.txt'), false);
});

test('restarts Astro only for configuration changes inside the site', () => {
	assert.equal(shouldRestartAstro('site', 'astro.config.ts'), true);
	assert.equal(shouldRestartAstro('site', 'package.json'), false);
	assert.equal(shouldRestartAstro('src', 'content.config.ts'), true);
	assert.equal(shouldRestartAstro('src', 'components/PageTitle.astro'), false);
});

test('restarts conservatively when a watcher omits the filename', () => {
	assert.equal(shouldRestartAstro('docs', null), true);
});
