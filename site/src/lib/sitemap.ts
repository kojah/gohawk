import { execFileSync } from 'node:child_process';
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import type { SitemapItem } from '@astrojs/sitemap';

const repositoryRoot = fileURLToPath(new URL('../../../', import.meta.url));
const docsRoot = path.join(repositoryRoot, 'docs');
const blogRoot = path.join(repositoryRoot, 'site/src/content/blog');
const pagesRoot = path.join(repositoryRoot, 'site/src/pages');

export function serializeSitemapItem(item: SitemapItem): SitemapItem {
	const lastmod = lastModifiedForURL(item.url);
	return lastmod ? { ...item, lastmod: lastmod.toISOString() } : item;
}

function lastModifiedForURL(rawURL: string): Date | undefined {
	const pathname = decodeURIComponent(new URL(rawURL).pathname);
	const sources = sourceFilesForPath(pathname).filter(existsSync);
	const timestamps = sources
		.map(lastModifiedForFile)
		.filter((date): date is Date => date !== undefined);
	if (timestamps.length === 0) return undefined;
	return new Date(Math.max(...timestamps.map((date) => date.getTime())));
}

function sourceFilesForPath(pathname: string): string[] {
	const slug = pathname.replace(/^\/+|\/+$/g, '');
	if (slug === '') return [path.join(docsRoot, 'index.md')];

	if (slug === 'blog') {
		return [path.join(pagesRoot, 'blog/index.astro'), ...publishedBlogFiles()];
	}

	if (slug.startsWith('blog/')) {
		const postSlug = slug.slice('blog/'.length);
		return contentCandidates(blogRoot, postSlug);
	}

	return [
		...contentCandidates(docsRoot, slug),
		path.join(pagesRoot, `${slug}.astro`),
		path.join(pagesRoot, slug, 'index.astro'),
	];
}

function contentCandidates(root: string, slug: string): string[] {
	return [
		path.join(root, `${slug}.md`),
		path.join(root, `${slug}.mdx`),
		path.join(root, slug, 'index.md'),
		path.join(root, slug, 'index.mdx'),
	];
}

function publishedBlogFiles(): string[] {
	return markdownFiles(blogRoot).filter(
		(file) => !/^---[\s\S]*?^draft:\s*true\s*$[\s\S]*?^---/m.test(readFileSync(file, 'utf8')),
	);
}

function markdownFiles(directory: string): string[] {
	return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
		const entryPath = path.join(directory, entry.name);
		if (entry.isDirectory()) return markdownFiles(entryPath);
		return /\.mdx?$/.test(entry.name) ? [entryPath] : [];
	});
}

function lastModifiedForFile(file: string): Date | undefined {
	const relativePath = path.relative(repositoryRoot, file);
	try {
		const committedAt = execFileSync('git', ['log', '-1', '--format=%cI', '--', relativePath], {
			cwd: repositoryRoot,
			encoding: 'utf8',
		}).trim();
		if (committedAt !== '') return new Date(committedAt);
	} catch {
		// Source archives may not include Git history; their file times are the best available evidence.
	}
	return new Date(statSync(file).mtimeMs);
}
