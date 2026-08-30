import assert from 'node:assert/strict';
import test from 'node:test';
import { shouldRestartAstro } from './dev-review.ts';

test('lets Astro hot-reload edits to existing documentation', () => {
	assert.equal(shouldRestartAstro('docs', 'change', 'configuration.md'), false);
	assert.equal(shouldRestartAstro('docs', 'change', 'analyzers/lockorder.mdx'), false);
});

test('restarts Astro for structural documentation changes', () => {
	assert.equal(shouldRestartAstro('docs', 'rename', 'configuration.md'), true);
	assert.equal(shouldRestartAstro('docs', 'rename', 'analyzers/lockorder.mdx'), true);
	assert.equal(shouldRestartAstro('docs', 'rename', 'notes.txt'), false);
});

test('restarts Astro only for configuration changes inside the site', () => {
	assert.equal(shouldRestartAstro('site', 'change', 'astro.config.ts'), true);
	assert.equal(shouldRestartAstro('site', 'change', 'package.json'), false);
	assert.equal(shouldRestartAstro('src', 'change', 'content.config.ts'), true);
	assert.equal(shouldRestartAstro('src', 'change', 'components/PageTitle.astro'), false);
});

test('restarts conservatively when a watcher omits the filename', () => {
	assert.equal(shouldRestartAstro('docs', 'change', null), true);
});
