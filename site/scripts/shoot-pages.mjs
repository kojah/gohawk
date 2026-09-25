import { existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import path from 'node:path';
import { parseArgs } from 'node:util';
import puppeteer from 'puppeteer-core';
import { siteDirectory, startPreview } from './preview-server.mjs';

// Screenshots built pages at phone, tablet, and desktop widths and reports layout
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

// The site loads Fraunces, Newsreader, and IBM Plex Mono from Google Fonts.
// The headless shell does not draw Newsreader's variable font, and may not
// reach Google at all, and a machine without system fonts has no fallback, so
// text vanishes. Register static builds of the same families from Fontsource,
// cached under the output directory, under the site's own family names, and
// remove the Google Fonts stylesheet so they replace the remote faces; the
// site's CSS is used unchanged. If they cannot be fetched, the page keeps its remote fonts
// and the report says whether text rendered.
const staticFonts = [
	['Fraunces', 'fraunces', [600, 700]],
	['Newsreader', 'newsreader', [400, 600]],
	['IBM Plex Mono', 'ibm-plex-mono', [400, 600]],
];

const { values } = parseArgs({
	options: {
		pages: { type: 'string' },
		widths: { type: 'string', default: '390,768,1280' },
		scale: { type: 'string', default: '2' },
		selector: { type: 'string' },
		out: { type: 'string', default: path.join(siteDirectory, '..', '.build', 'site-shots') },
	},
});
const pages = values.pages ? values.pages.split(',').filter(Boolean) : defaultPages;
const widths = values.widths.split(',').map(Number);
mkdirSync(values.out, { recursive: true });

const fontStyles = await staticFontStyles(path.join(values.out, 'fonts'));
const chrome = findBrowser();
const preview = await startPreview({ quiet: true });
// With static fonts in hand, make Google's font hosts unreachable so the
// site's font stylesheet never loads. Whether it loaded before a capture
// otherwise decides which faces the page uses, and text vanishes at random.
const blockGoogleFonts = fontStyles
	? ['--host-resolver-rules=MAP fonts.googleapis.com ~NOTFOUND, MAP fonts.gstatic.com ~NOTFOUND']
	: [];
const browser = await puppeteer.launch({
	executablePath: chrome.executablePath,
	headless: chrome.headless,
	env: chrome.env,
	args: ['--no-sandbox', ...blockGoogleFonts],
});
let problems = 0;
let textRendered = true;
try {
	const page = await browser.newPage();

	for (const width of widths) {
		await page.setViewport({ width, height: 900, deviceScaleFactor: Number(values.scale) });
		for (const pathname of pages) {
			await page.goto(preview.origin + pathname, { waitUntil: 'networkidle0' });
			if (fontStyles) {
				// Drop the Google Fonts stylesheet so only the static faces remain.
				await page.evaluate(() => {
					for (const link of document.querySelectorAll('link[href*="fonts.googleapis.com"]'))
						link.remove();
				});
				await page.addStyleTag({ content: fontStyles });
			}
			// Web fonts load only once text uses them, so load every face now;
			// otherwise the render probe below can run before its font arrives.
			await page.evaluate(() =>
				Promise.all([...document.fonts].map((face) => face.load().catch(() => undefined))),
			);
			await page.evaluate(() => document.fonts.ready);
			if (fontStyles) await waitForStaticFonts(page);
			// Let the page repaint with the fonts it just loaded before capturing.
			await page.evaluate(
				() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
			);
			await new Promise((resolve) => setTimeout(resolve, 300));
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
			'\nRun scripts/site-shot-deps.sh, or install fontconfig and a font package, for readable screenshots.',
	);
}
console.log(`\n${problems} layout problem${problems === 1 ? '' : 's'}. Screenshots: ${values.out}`);
if (problems > 0) process.exitCode = 1;

// waitForStaticFonts loads every substituted face and polls until the browser
// reports each one usable, so a capture never catches text mid-load.
async function waitForStaticFonts(page) {
	const descriptors = staticFonts.flatMap(([familyName, , weights]) =>
		weights.flatMap((weight) => [
			`${weight} 16px "${familyName}"`,
			`italic ${weight} 16px "${familyName}"`,
		]),
	);
	const ready = await page.evaluate(async (fonts) => {
		for (let attempt = 0; attempt < 50; attempt++) {
			await Promise.all(fonts.map((font) => document.fonts.load(font).catch(() => [])));
			if (fonts.every((font) => document.fonts.check(font))) return true;
			await new Promise((resolve) => setTimeout(resolve, 100));
		}
		return false;
	}, descriptors);
	if (!ready) console.log('  static fonts did not finish loading; text may be missing');
}

async function staticFontStyles(cache) {
	mkdirSync(cache, { recursive: true });
	const faces = [];
	try {
		for (const [familyName, family, weights] of staticFonts) {
			for (const weight of weights) {
				for (const style of ['normal', 'italic']) {
					const file = path.join(cache, `${family}-${weight}-${style}.woff2`);
					if (!existsSync(file)) {
						const url = `https://cdn.jsdelivr.net/npm/@fontsource/${family}/files/${family}-latin-${weight}-${style}.woff2`;
						const response = await fetch(url);
						if (!response.ok) continue;
						writeFileSync(file, Buffer.from(await response.arrayBuffer()));
					}
					const data = readFileSync(file).toString('base64');
					faces.push(
						`@font-face{font-family:"${familyName}";font-weight:${weight};font-style:${style};` +
							`src:url(data:font/woff2;base64,${data}) format("woff2")}`,
					);
				}
			}
		}
	} catch (error) {
		console.log(`Static fonts unavailable (${error.message}); using the page's own fonts.`);
		return undefined;
	}
	return faces.join('');
}

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

// findBrowser prefers an explicit path, then the full Chrome build with the
// libraries and fonts scripts/site-shot-deps.sh unpacks, then a headless
// shell that Puppeteer or Playwright already downloaded, then a system
// Chrome. Without fontconfig and a system font, Chrome may not draw text, so
// the headless shell alone can produce text-less captures at some widths.
function findBrowser() {
	const explicit = process.env.GOHAWK_SHOT_BROWSER || process.env.PUPPETEER_EXECUTABLE_PATH;
	if (explicit) return { executablePath: explicit, headless: 'shell', env: process.env };
	const deps = path.join(siteDirectory, '..', '.build', 'chrome-deps', 'root');
	const fullChrome = [
		path.join(homedir(), '.cache', 'puppeteer', 'chrome'),
		path.join(homedir(), '.cache', 'ms-playwright'),
	]
		.map((root) => findFile(root, 'chrome', 4))
		.find(Boolean);
	if (fullChrome && existsSync(path.join(deps, 'fonts.conf'))) {
		const libraries = path.join(deps, 'usr', 'lib', 'x86_64-linux-gnu');
		return {
			executablePath: fullChrome,
			headless: true,
			env: {
				...process.env,
				LD_LIBRARY_PATH: [libraries, process.env.LD_LIBRARY_PATH].filter(Boolean).join(':'),
				FONTCONFIG_FILE: path.join(deps, 'fonts.conf'),
			},
		};
	}
	const shell = [
		path.join(homedir(), '.cache', 'puppeteer', 'chrome-headless-shell'),
		path.join(homedir(), '.cache', 'ms-playwright'),
	]
		.map((root) => findFile(root, 'chrome-headless-shell', 4))
		.find(Boolean);
	if (shell) return { executablePath: shell, headless: 'shell', env: process.env };
	for (const candidate of [
		'/usr/bin/chromium',
		'/usr/bin/chromium-browser',
		'/usr/bin/google-chrome',
	]) {
		if (existsSync(candidate))
			return { executablePath: candidate, headless: true, env: process.env };
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
