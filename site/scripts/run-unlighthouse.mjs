import { spawn } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { createServer } from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const siteDirectory = fileURLToPath(new URL('../', import.meta.url));
const distDirectory = path.join(siteDirectory, 'dist');
const host = '127.0.0.1';
const production = process.argv.includes('--production');
const port = production ? undefined : await availablePort(host);
const origin = production ? 'https://gohawk.dev' : `http://${host}:${port}`;
const outputDirectory = path.join(
	siteDirectory,
	production ? '.unlighthouse-production' : '.unlighthouse',
);

const paths = production ? await productionSitemapPaths(origin) : localSitemapPaths();
if (paths.length === 0) throw new Error('The sitemap contains no auditable pages.');
if (paths.some((pathname) => pathname.includes(',')))
	throw new Error('Unlighthouse cannot receive sitemap paths containing commas.');

const preview = production
	? undefined
	: spawn(
			process.execPath,
			[
				path.join(siteDirectory, 'node_modules/astro/bin/astro.mjs'),
				'preview',
				'--host',
				host,
				'--port',
				`${port}`,
			],
			{ cwd: siteDirectory, stdio: 'inherit' },
		);

try {
	if (preview) await waitForServer(origin, preview);
	console.log(`Auditing ${paths.length} sitemap pages at ${origin}.`);
	const exitCode = await run(
		process.execPath,
		[
			path.join(siteDirectory, 'node_modules/unlighthouse-ci/bin/unlighthouse-ci.mjs'),
			'--site',
			origin,
			'--urls',
			paths.join(','),
			'--config-file',
			path.join(siteDirectory, 'unlighthouse.config.ts'),
			'--output-path',
			outputDirectory,
		],
		siteDirectory,
	);
	if (exitCode !== 0) process.exitCode = exitCode;
	else validateReport(outputDirectory, paths);
} finally {
	if (preview) await stop(preview);
}

function localSitemapPaths() {
	const index = readFileSync(path.join(distDirectory, 'sitemap-index.xml'), 'utf8');
	const sitemapFiles = sitemapLocations(index, 'https://gohawk.dev').map((location) => {
		const url = new URL(location);
		return path.join(distDirectory, path.basename(url.pathname));
	});
	const pages = sitemapFiles.flatMap((file) => elements(readFileSync(file, 'utf8'), 'loc'));
	return pagePaths(pages, 'https://gohawk.dev');
}

async function productionSitemapPaths(siteOrigin) {
	const index = await fetchText(`${siteOrigin}/sitemap-index.xml`);
	const sitemapURLs = sitemapLocations(index, siteOrigin);
	const sitemaps = await Promise.all(sitemapURLs.map(fetchText));
	return pagePaths(
		sitemaps.flatMap((sitemap) => elements(sitemap, 'loc')),
		siteOrigin,
	);
}

function sitemapLocations(index, expectedOrigin) {
	return elements(index, 'loc').map((location) => {
		if (new URL(location).origin !== expectedOrigin)
			throw new Error(`Unexpected sitemap origin: ${location}`);
		return location;
	});
}

function pagePaths(pages, expectedOrigin) {
	return [
		...new Set(
			pages.map((location) => {
				const url = new URL(location);
				if (url.origin !== expectedOrigin) throw new Error(`Unexpected page origin: ${location}`);
				return `${url.pathname}${url.search}`;
			}),
		),
	];
}

async function fetchText(url) {
	const response = await fetch(url);
	if (!response.ok) throw new Error(`Failed to fetch ${url}: HTTP ${response.status}`);
	return response.text();
}

function validateReport(directory, expectedPaths) {
	const reportPath = path.join(directory, 'ci-result.json');
	const report = JSON.parse(readFileSync(reportPath, 'utf8'));
	const actualPaths = new Set(report.routes.map((route) => route.path));
	const expected = new Set(expectedPaths);
	const missing = expectedPaths.filter((pathname) => !actualPaths.has(pathname));
	const unexpected = [...actualPaths].filter((pathname) => !expected.has(pathname));
	if (missing.length > 0 || unexpected.length > 0) {
		throw new Error(
			[
				'Unlighthouse report does not match the sitemap.',
				...missing.map((pathname) => `Missing report: ${pathname}`),
				...unexpected.map((pathname) => `Unexpected report: ${pathname}`),
			].join('\n'),
		);
	}
	console.log(`Verified Lighthouse reports for all ${expected.size} sitemap pages.`);
}

function elements(xml, name) {
	return [...xml.matchAll(new RegExp(`<${name}>(.*?)<\\/${name}>`, 'gs'))].map((match) =>
		decodeXML(match[1]),
	);
}

function decodeXML(value) {
	return value
		.replaceAll('&amp;', '&')
		.replaceAll('&lt;', '<')
		.replaceAll('&gt;', '>')
		.replaceAll('&quot;', '"')
		.replaceAll('&apos;', "'");
}

async function waitForServer(url, child) {
	for (let attempt = 0; attempt < 60; attempt++) {
		if (child.exitCode !== null)
			throw new Error(`Astro preview exited before it became ready (code ${child.exitCode}).`);
		try {
			const response = await fetch(url);
			if (response.ok) return;
		} catch {
			// The preview server has not bound its socket yet.
		}
		await new Promise((resolve) => setTimeout(resolve, 500));
	}
	throw new Error(`Timed out waiting for Astro preview at ${url}.`);
}

function availablePort(hostname) {
	return new Promise((resolve, reject) => {
		const server = createServer();
		server.once('error', reject);
		server.listen(0, hostname, () => {
			const address = server.address();
			if (typeof address === 'string' || address === null) {
				server.close();
				reject(new Error('Failed to allocate a local port for Astro preview.'));
				return;
			}
			server.close((error) => {
				if (error) reject(error);
				else resolve(address.port);
			});
		});
	});
}

function run(command, args, cwd) {
	return new Promise((resolve, reject) => {
		const child = spawn(command, args, { cwd, stdio: 'inherit' });
		child.once('error', reject);
		child.once('close', (code, signal) => {
			if (signal) reject(new Error(`Unlighthouse was terminated by ${signal}.`));
			else resolve(code ?? 1);
		});
	});
}

async function stop(child) {
	if (child.exitCode !== null) return;
	child.kill('SIGTERM');
	await Promise.race([
		new Promise((resolve) => child.once('close', resolve)),
		new Promise((resolve) => setTimeout(resolve, 5_000)),
	]);
	if (child.exitCode === null) child.kill('SIGKILL');
}
