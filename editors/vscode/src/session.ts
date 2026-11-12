// Owns the running `gohawk lsp` process: starting it, stopping it, and
// sending it commands. Everything the server says is validated before it
// reaches the rest of the extension.

import type * as vscode from 'vscode';
import {
	LanguageClient,
	type LanguageClientOptions,
	type ServerOptions,
	State,
} from 'vscode-languageclient/node';
import type { Binary } from './install.ts';
import { parseStatus, type Status } from './protocol.ts';

const settingKeys = [
	'run',
	'debounce',
	'scope',
	'experimental',
	'memoryLimit',
	'maxParallel',
	'timeout',
] as const;

export type SessionOptions = {
	output: vscode.OutputChannel;
	/** Commands this extension implements for the server's code actions. */
	clientCommands: readonly string[];
	readSettings: () => vscode.WorkspaceConfiguration;
	onStatus: (status: Status) => void;
};

export class Session implements vscode.Disposable {
	private client: LanguageClient | undefined;
	private readonly options: SessionOptions;

	constructor(options: SessionOptions) {
		this.options = options;
	}

	get running(): boolean {
		return this.client !== undefined;
	}

	async start(binary: Binary): Promise<void> {
		await this.stop();
		const configuration = this.options.readSettings();
		const settings = Object.fromEntries(settingKeys.map((key) => [key, configuration.get(key)]));
		const eventLog = configuration.get<string>('eventLog', '').trim();
		const serverOptions: ServerOptions = {
			command: binary.path,
			args: eventLog === '' ? ['lsp'] : ['lsp', '-event-log', eventLog],
		};
		const clientOptions: LanguageClientOptions = {
			documentSelector: [{ scheme: 'file', language: 'go' }],
			outputChannel: this.options.output,
			synchronize: { configurationSection: 'gohawk' },
			initializationOptions: { settings, clientCommands: this.options.clientCommands },
		};
		const client = new LanguageClient('gohawk', 'gohawk', serverOptions, clientOptions);
		client.onNotification('gohawk/status', (params: unknown) => {
			const status = parseStatus(params);
			if (status !== undefined) {
				this.options.onStatus(status);
			}
		});
		client.onDidChangeState((event) => {
			if (event.newState === State.Stopped && this.client === client) {
				this.options.onStatus({
					state: 'error',
					text: 'gohawk: server stopped',
					tooltip: 'Run “gohawk: Restart Server” to start it again.',
					findings: 0,
				});
			}
		});
		this.options.output.appendLine(`gohawk binary: ${binary.path} (${binary.source})`);
		this.client = client;
		try {
			await client.start();
		} catch (error) {
			this.client = undefined;
			throw error;
		}
	}

	async stop(): Promise<void> {
		const client = this.client;
		this.client = undefined;
		if (client === undefined) {
			return;
		}
		try {
			await client.stop();
		} catch {
			// The process may already be gone; there is nothing left to stop.
		}
	}

	/** Runs a server command. Rejects when the server is not running or refuses. */
	async command(name: string, ...argumentList: unknown[]): Promise<unknown> {
		if (this.client === undefined) {
			throw new Error('gohawk is not running');
		}
		return this.client.sendRequest('workspace/executeCommand', {
			command: name,
			arguments: argumentList,
		});
	}

	dispose(): void {
		void this.stop();
	}
}
