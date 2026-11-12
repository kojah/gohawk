// Fonts for screenshots. A headless Linux machine usually has only fallback
// fonts, which makes VS Code look unlike it does on a desktop. This fetches
// Inter and JetBrains Mono once and points the browser's font configuration
// at them, without installing anything on the machine. Other platforms have
// good fonts already and are left alone.

import { execFile } from 'node:child_process';
import { access, copyFile, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';

const run = promisify(execFile);

const sources = [
	{
		url: 'https://github.com/rsms/inter/releases/download/v4.1/Inter-4.1.zip',
		files: ['InterVariable.ttf', 'InterVariable-Italic.ttf'],
	},
	{
		url: 'https://github.com/JetBrains/JetBrainsMono/releases/download/v2.304/JetBrainsMono-2.304.zip',
		files: [
			'fonts/variable/JetBrainsMono[wght].ttf',
			'fonts/variable/JetBrainsMono-Italic[wght].ttf',
		],
	},
];

const interface_ = 'Inter Variable';
const monospace = 'JetBrains Mono';

/** The editor font the screenshots use, or undefined to keep VS Code's default. */
export const editorFont = process.platform === 'linux' ? monospace : undefined;

function alias(family, preferred) {
	return `<alias binding="strong"><family>${family}</family><prefer><family>${preferred}</family></prefer></alias>`;
}

/**
 * Returns environment variables that make the browser use the screenshot
 * fonts, downloading them into cacheDirectory the first time.
 */
export async function screenshotFontEnvironment(cacheDirectory) {
	if (process.platform !== 'linux') {
		return {};
	}
	const files = join(cacheDirectory, 'files');
	const configuration = join(cacheDirectory, 'fonts.conf');
	const ready = await access(configuration).then(
		() => true,
		() => false,
	);
	if (!ready) {
		await mkdir(files, { recursive: true });
		const staging = await mkdtemp(join(tmpdir(), 'gohawk-fonts-'));
		try {
			for (const [index, source] of sources.entries()) {
				const response = await fetch(source.url, { signal: AbortSignal.timeout(180_000) });
				if (!response.ok) {
					throw new Error(`download ${source.url}: HTTP ${response.status}`);
				}
				const archive = join(staging, `fonts-${index}.zip`);
				await writeFile(archive, Buffer.from(await response.arrayBuffer()));
				const unpacked = join(staging, String(index));
				await run('unzip', ['-q', archive, '-d', unpacked]);
				for (const file of source.files) {
					await copyFile(join(unpacked, file), join(files, file.split('/').at(-1)));
				}
			}
		} finally {
			await rm(staging, { recursive: true, force: true });
		}
		const sansFamilies = [
			'system-ui',
			'sans-serif',
			'Ubuntu',
			'Droid Sans',
			'Segoe UI',
			'Helvetica',
			'Arial',
		];
		const monoFamilies = [
			'monospace',
			'Droid Sans Mono',
			'Consolas',
			'Courier New',
			'Menlo',
			'Monaco',
		];
		await writeFile(
			configuration,
			[
				'<?xml version="1.0"?>',
				'<!DOCTYPE fontconfig SYSTEM "fonts.dtd">',
				'<fontconfig>',
				`<dir>${files}</dir>`,
				`<cachedir>${join(cacheDirectory, 'cache')}</cachedir>`,
				'<include ignore_missing="yes">/etc/fonts/fonts.conf</include>',
				...sansFamilies.map((family) => alias(family, interface_)),
				...monoFamilies.map((family) => alias(family, monospace)),
				'</fontconfig>',
			].join('\n'),
		);
	}
	return { FONTCONFIG_FILE: configuration };
}
