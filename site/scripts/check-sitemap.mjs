import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const distDirectory = fileURLToPath(new URL('../dist/', import.meta.url));
const index = readFileSync(path.join(distDirectory, 'sitemap-index.xml'), 'utf8');
const sitemapURLs = elements(index, 'loc');
const seen = new Set();
const failures = [];
let entryCount = 0;

for (const sitemapURL of sitemapURLs) {
	const url = new URL(sitemapURL);
	if (url.origin !== 'https://gohawk.dev') {
		failures.push(`unexpected sitemap origin: ${sitemapURL}`);
		continue;
	}
	const sitemap = readFileSync(path.join(distDirectory, path.basename(url.pathname)), 'utf8');
	for (const entry of sitemap.matchAll(/<url>(.*?)<\/url>/gs)) {
		entryCount++;
		const pageURL = element(entry[1], 'loc') ?? '(missing loc)';
		const lastmod = element(entry[1], 'lastmod');
		if (seen.has(pageURL)) failures.push(`duplicate URL: ${pageURL}`);
		seen.add(pageURL);
		if (!lastmod) {
			failures.push(`missing lastmod: ${pageURL}`);
			continue;
		}
		const timestamp = Date.parse(lastmod);
		if (Number.isNaN(timestamp)) failures.push(`invalid lastmod for ${pageURL}: ${lastmod}`);
		if (timestamp > Date.now() + 5 * 60 * 1000)
			failures.push(`future lastmod for ${pageURL}: ${lastmod}`);
	}
}

if (entryCount === 0) failures.push('sitemap contains no URL entries');
if (failures.length > 0) {
	throw new Error(
		`Invalid sitemap metadata:\n${failures.map((failure) => `- ${failure}`).join('\n')}`,
	);
}

console.log(`Validated sitemap metadata for ${entryCount} URLs.`);

function elements(xml, name) {
	return [...xml.matchAll(new RegExp(`<${name}>(.*?)<\\/${name}>`, 'gs'))].map((match) =>
		decodeXML(match[1]),
	);
}

function element(xml, name) {
	return elements(xml, name)[0];
}

function decodeXML(value) {
	return value
		.replaceAll('&amp;', '&')
		.replaceAll('&lt;', '<')
		.replaceAll('&gt;', '>')
		.replaceAll('&quot;', '"')
		.replaceAll('&apos;', "'");
}
