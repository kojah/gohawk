import { Agentation } from 'agentation';
import { useEffect, useState } from 'react';

const AGENTATION_ENDPOINT = '/__agentation';
const REVIEW_ENDPOINT = '/__agentation-review';
const SETTINGS_KEY = 'feedback-toolbar-settings';

type AgentName = 'codex' | 'claude';

type WorkerStatus = {
	phase: 'idle' | 'queued' | 'working' | 'done' | 'failed';
	detail?: string;
	agent?: AgentName;
};

const AGENT_LABEL: Record<AgentName, string> = { codex: 'Codex', claude: 'Claude Code' };

function enableManualSendMode(): void {
	try {
		const saved = JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}') as Record<string, unknown>;
		localStorage.setItem(
			SETTINGS_KEY,
			JSON.stringify({ ...saved, webhookUrl: '', webhooksEnabled: false }),
		);
	} catch {
		localStorage.setItem(SETTINGS_KEY, JSON.stringify({ webhookUrl: '', webhooksEnabled: false }));
	}
}

export default function AgentationToolbar() {
	const [ready, setReady] = useState(false);
	const [status, setStatus] = useState<WorkerStatus>({ phase: 'idle' });
	const [agent, setAgent] = useState<AgentName>('codex');
	// Empty until the review supervisor answers, so the toggle never renders on
	// the deployed site, where the review endpoints do not exist.
	const [available, setAvailable] = useState<AgentName[]>([]);

	useEffect(() => {
		enableManualSendMode();
		setReady(true);

		// Load the current worker agent so the toggle reflects the running choice.
		fetch(`${REVIEW_ENDPOINT}/agent`)
			.then((response) => response.json())
			.then((body: { agent?: AgentName; available?: AgentName[] }) => {
				if (body.agent) setAgent(body.agent);
				if (Array.isArray(body.available) && body.available.length > 0) {
					setAvailable(body.available);
				}
			})
			.catch(() => {
				// The review supervisor is only present in `make site-review`.
			});

		const events = new EventSource(`${REVIEW_ENDPOINT}/events`);
		events.onmessage = (event) => {
			try {
				const next = JSON.parse(event.data) as WorkerStatus;
				setStatus(next);
				// A change made elsewhere (or another tab) keeps the toggle in sync.
				if (next.agent) setAgent(next.agent);
			} catch {
				// Ignore malformed development-tool status events.
			}
		};
		return () => events.close();
	}, []);

	async function selectAgent(next: AgentName): Promise<void> {
		if (next === agent) return;
		const previous = agent;
		setAgent(next); // optimistic; reconciled by the response and the SSE status
		try {
			const response = await fetch(`${REVIEW_ENDPOINT}/agent`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ agent: next }),
			});
			const body = (await response.json()) as { agent?: AgentName };
			if (body.agent) setAgent(body.agent);
			else setAgent(previous);
		} catch {
			setAgent(previous);
		}
	}

	const webhookUrl = `${window.location.origin}${REVIEW_ENDPOINT}/webhook`;
	const busy = status.phase === 'queued' || status.phase === 'working';
	return (
		<>
			{ready && available.length > 1 && (
				<fieldset
					// Mark our own review controls the way agentation marks its UI, so its
					// element picker excludes them (it guards on closest [data-agentation-root]).
					data-agentation-root=""
					aria-label="Review agent"
					title={
						busy ? 'Applies to the next batch; a run is in progress' : 'Agent that applies feedback'
					}
					style={{
						position: 'fixed',
						right: '1.25rem',
						bottom: '7.75rem',
						zIndex: 100000,
						display: 'flex',
						alignItems: 'center',
						gap: '0.35rem',
						margin: 0,
						minInlineSize: 0,
						border: 'none',
						padding: '0.3rem 0.4rem',
						borderRadius: '999px',
						background: '#1a1a1a',
						color: '#fff',
						font: '500 12px/1.2 system-ui, sans-serif',
						boxShadow: '0 2px 8px rgb(0 0 0 / 20%)',
					}}
				>
					<span style={{ opacity: 0.7, paddingLeft: '0.25rem' }}>Agent</span>
					{available.map((candidate) => {
						const selected = candidate === agent;
						return (
							<button
								key={candidate}
								type="button"
								aria-pressed={selected}
								onClick={() => void selectAgent(candidate)}
								style={{
									cursor: 'pointer',
									border: 'none',
									borderRadius: '999px',
									padding: '0.25rem 0.6rem',
									font: '500 12px/1.2 system-ui, sans-serif',
									background: selected ? '#f5f5f5' : 'transparent',
									color: selected ? '#111' : '#ccc',
								}}
							>
								{AGENT_LABEL[candidate]}
							</button>
						);
					})}
				</fieldset>
			)}
			{status.phase !== 'idle' && (
				<div
					data-agentation-root=""
					role="status"
					title={status.detail}
					style={{
						position: 'fixed',
						right: '1.25rem',
						bottom: '4.75rem',
						zIndex: 100000,
						padding: '0.4rem 0.7rem',
						borderRadius: '999px',
						background: status.phase === 'failed' ? '#7f1d1d' : '#1a1a1a',
						color: '#fff',
						font: '500 12px/1.2 system-ui, sans-serif',
						boxShadow: '0 2px 8px rgb(0 0 0 / 20%)',
					}}
				>
					{AGENT_LABEL[status.agent ?? agent]}: {status.phase}
				</div>
			)}
			{ready && <Agentation endpoint={AGENTATION_ENDPOINT} webhookUrl={webhookUrl} />}
		</>
	);
}
