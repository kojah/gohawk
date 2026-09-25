import { existsSync, mkdirSync, readdirSync } from 'node:fs';
import { homedir } from 'node:os';
import path from 'node:path';
import { parseArgs } from 'node:util';
import puppeteer from 'puppeteer-core';
import { siteDirectory, startPreview } from './preview-server.mjs';

// Screenshots built pages at phone and desktop widths and reports layout
// problems a reviewer would otherwise need to eyeball: content wider than the
// screen, identifiers split mid-word, and text that failed to render.
// Run it after `make site-build`, usually through `make site-shot`.

const defaultPages = [
	'/',
	'/faq/',
	'/configuration/',
	'/understanding-ssa/',
	'/analyzers/resources-and-lifecycle/resourcelifetime/',
	'/analyzers/concurrency-and-synchronization/lockorder/',
];

const { values } = parseArgs({
	options: {
		pages: { type: 'string' },
		widths: { type: 'string', default: '390,1280' },
		selector: { type: 'string' },
		out: { type: 'string', default: path.join(siteDirectory, '..', '.build', 'site-shots') },
	},
});
const pages = values.pages ? values.pages.split(',').filter(Boolean) : defaultPages;
const widths = values.widths.split(',').map(Number);
mkdirSync(values.out, { recursive: true });

const executablePath = findBrowser();
const preview = await startPreview({ quiet: true });
const browser = await puppeteer.launch({
	executablePath,
	headless: 'shell',
	args: ['--no-sandbox'],
});
let problems = 0;
let textRendered = true;
try {
	const page = await browser.newPage();
	for (const width of widths) {
		await page.setViewport({ width, height: 900 });
		for (const pathname of pages) {
			await page.goto(preview.origin + pathname, { waitUntil: 'networkidle0' });
			await page.evaluate(() => document.fonts.ready);
			const name = `${slug(pathname)}-${width}.png`;
			await screenshot(page, path.join(values.out, name), values.selector);
			const report = await page.evaluate(measureLayout);
			textRendered &&= report.textRendered;
			const findings = [
				...report.overflow.map((element) => `wider than the screen: ${element}`),
				...report.splitIdentifiers.map((text) => `identifier split mid-word: ${text}`),
			];
			problems += findings.length;
			console.log(`${width}px ${pathname} -> ${name}${findings.length ? '' : ' ok'}`);
			for (const finding of findings) console.log(`  ${finding}`);
		}
	}
} finally {
	await browser.close();
	await preview.stop();
}
if (!textRendered) {
	console.log(
		'\nText did not render, so screenshots show layout only and the split-word check was skipped.' +
			'\nInstall fontconfig and a font package (for example fonts-dejavu-core) for readable screenshots.',
	);
}
console.log(`\n${problems} layout problem${problems === 1 ? '' : 's'}. Screenshots: ${values.out}`);
if (problems > 0) process.exitCode = 1;

async function screenshot(page, file, selector) {
	if (!selector) {
		await page.screenshot({ path: file, fullPage: true });
		return;
	}
	const element = await page.$(selector);
	if (!element) throw new Error(`No element matches ${selector}`);
	await element.screenshot({ path: file });
}

// measureLayout runs in the page. It must not reference anything outside itself.
function measureLayout() {
	const probe = document.createElement('span');
	probe.textContent = 'gohawk';
	document.body.append(probe);
	const textRendered = probe.getBoundingClientRect().width > 0;
	probe.remove();

	const describe = (element) =>
		element.id
			? `#${element.id}`
			: `${element.tagName.toLowerCase()}.${[...element.classList].join('.')}`;
	// Only a page that actually scrolls sideways has an overflow problem;
	// fixed sidebars may sit past the edge by design.
	const root = document.documentElement;
	const limit = root.clientWidth + 1;
	const scrollsSideways = root.scrollWidth > limit;
	const overflow = [...(scrollsSideways ? document.querySelectorAll('body *') : [])]
		.filter((element) => {
			const box = element.getBoundingClientRect();
			if (box.width === 0 || box.right <= limit) return false;
			// A scroll container that holds its own overflow is fine.
			for (let parent = element.parentElement; parent; parent = parent.parentElement) {
				const style = getComputedStyle(parent);
				if (/(auto|scroll|hidden)/.test(style.overflowX)) return false;
			}
			return true;
		})
		.slice(0, 5)
		.map(describe);

	const splitIdentifiers = textRendered
		? [...document.querySelectorAll('code')]
				.filter((code) => !code.closest('pre') && !/\s/.test(code.textContent.trim()))
				.filter((code) => {
					const range = document.createRange();
					range.selectNodeContents(code);
					const lines = [...range.getClientRects()].map((rect) => Math.round(rect.top));
					if (new Set(lines).size < 2) return false;
					// Wrapping at a hyphen, slash, dot, or comma is acceptable; anywhere else
					// is a split word.
					const text = code.textContent.trim();
					return !/[-/._,]/.test(text);
				})
				.slice(0, 5)
				.map((code) => code.textContent.trim())
		: [];
	return { textRendered, overflow, splitIdentifiers };
}

function slug(pathname) {
	return pathname.replace(/^\/|\/$/g, '').replaceAll('/', '-') || 'index';
}

// findBrowser prefers an explicit path, then a headless shell or Chrome that
// Puppeteer or Playwright already downloaded, then a system Chrome.
function findBrowser() {
	const explicit = process.env.GOHAWK_SHOT_BROWSER || process.env.PUPPETEER_EXECUTABLE_PATH;
	if (explicit) return explicit;
	const caches = [
		[path.join(homedir(), '.cache', 'puppeteer', 'chrome-headless-shell'), 'chrome-headless-shell'],
		[path.join(homedir(), '.cache', 'ms-playwright'), 'chrome-headless-shell'],
	];
	for (const [root, binary] of caches) {
		const found = findFile(root, binary, 4);
		if (found) return found;
	}
	for (const candidate of [
		'/usr/bin/chromium',
		'/usr/bin/chromium-browser',
		'/usr/bin/google-chrome',
	]) {
		if (existsSync(candidate)) return candidate;
	}
	throw new Error(
		'No headless Chrome found. Install one with ' +
			'`pnpm --dir site exec puppeteer browsers install chrome-headless-shell@stable`, ' +
			'or set GOHAWK_SHOT_BROWSER to a Chrome binary.',
	);
}

function findFile(root, name, depth) {
	if (depth < 0 || !existsSync(root)) return undefined;
	for (const entry of readdirSync(root, { withFileTypes: true })) {
		const full = path.join(root, entry.name);
		if (entry.isFile() && entry.name === name) return full;
		if (entry.isDirectory()) {
			const found = findFile(full, name, depth - 1);
			if (found) return found;
		}
	}
	return undefined;
}
