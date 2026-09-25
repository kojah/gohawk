import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Serves the built site with `astro preview` on a free local port for tools
// that need a real page: the Lighthouse audit and the page screenshots.

export const siteDirectory = fileURLToPath(new URL('../', import.meta.url));

// startPreview serves dist/ and resolves once the server answers. Call stop()
// when done; it also runs if the caller throws.
export async function startPreview({ quiet = false } = {}) {
	const host = '127.0.0.1';
	const port = await availablePort(host);
	const origin = `http://${host}:${port}`;
	const child = spawn(
		process.execPath,
		[
			path.join(siteDirectory, 'node_modules/astro/bin/astro.mjs'),
			'preview',
			'--host',
			host,
			'--port',
			`${port}`,
		],
		{ cwd: siteDirectory, stdio: quiet ? 'ignore' : 'inherit' },
	);
	const stop = async () => {
		await stopChild(child);
		// Astro detaches the preview server into the background when it detects
		// an AI agent (the am-i-vibing check), and the process spawned here then
		// exits at once. Stop that background server too, or it outlives the run.
		if (child.exitCode === 0) await stopBackground();
	};
	try {
		await waitForServer(origin, child);
	} catch (error) {
		await stop();
		throw error;
	}
	return { origin, stop };
}

function stopBackground() {
	return new Promise((resolve) => {
		const stopper = spawn(
			process.execPath,
			[path.join(siteDirectory, 'node_modules/astro/bin/astro.mjs'), 'preview', 'stop'],
			{ cwd: siteDirectory, stdio: 'ignore' },
		);
		stopper.once('close', resolve);
		stopper.once('error', resolve);
	});
}

async function waitForServer(url, child) {
	for (let attempt = 0; attempt < 60; attempt++) {
		// Exit code 0 means Astro handed the server to a background process;
		// keep polling the port. Any other exit is a failure.
		if (child.exitCode !== null && child.exitCode !== 0)
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

async function stopChild(child) {
	if (child.exitCode !== null) return;
	child.kill('SIGTERM');
	await Promise.race([
		new Promise((resolve) => child.once('close', resolve)),
		new Promise((resolve) => setTimeout(resolve, 5_000)),
	]);
	if (child.exitCode === null) child.kill('SIGKILL');
}
