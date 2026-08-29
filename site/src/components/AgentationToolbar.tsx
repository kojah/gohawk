import { Agentation } from 'agentation';
import { useEffect, useState } from 'react';

const AGENTATION_ENDPOINT = '/__agentation';
const REVIEW_ENDPOINT = '/__agentation-review';
const SETTINGS_KEY = 'feedback-toolbar-settings';

type WorkerStatus = {
	phase: 'idle' | 'queued' | 'working' | 'done' | 'failed';
	detail?: string;
};

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

	useEffect(() => {
		enableManualSendMode();
		setReady(true);
		const events = new EventSource(`${REVIEW_ENDPOINT}/events`);
		events.onmessage = (event) => {
			try {
				setStatus(JSON.parse(event.data) as WorkerStatus);
			} catch {
				// Ignore malformed development-tool status events.
			}
		};
		return () => events.close();
	}, []);

	const webhookUrl = `${window.location.origin}${REVIEW_ENDPOINT}/webhook`;
	return (
		<>
			{status.phase !== 'idle' && (
				<div
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
					Codex: {status.phase}
				</div>
			)}
			{ready && <Agentation endpoint={AGENTATION_ENDPOINT} webhookUrl={webhookUrl} />}
		</>
	);
}
