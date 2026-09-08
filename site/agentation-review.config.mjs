import { defineConfig } from 'agentation-review';

export default defineConfig({
	projectDirectory: '..',
	projectName: 'gohawk',
	runtimeDirectory: '.agentation-runtime',
	availableAgents: ['codex', 'claude'],
	defaultAgent: 'codex',
	devServer: {
		command: 'astro',
		args: ['dev'],
		cwd: '.',
		env: {
			// Astro otherwise auto-detaches in detected AI environments. Keeping it
			// in the foreground lets agentation-review own the development process.
			ASTRO_DEV_BACKGROUND: '1',
		},
	},
	codex: { approveForMe: true },
	claude: { dangerouslySkipPermissions: true },
	workerInstructions:
		'Do not commit or push. Do not modify the Agentation review integration unless the feedback explicitly concerns it.',
});
