import type {
	ClientMessage,
	ServerMessage,
	SnapshotTasksData,
	DeltaTasksData,
	SnapshotPlanningData,
	DeltaPlanningData
} from './types';

const RECONNECT_MIN = 1000;
const RECONNECT_MAX = 30000;

type MessageKey = `${'snapshot' | 'delta'}:${'tasks' | 'planning'}`;
type MessageHandler = (data: unknown) => void;

function createWSClient() {
	let connected = $state(false);

	let socket: WebSocket | null = null;
	let reconnectDelay = RECONNECT_MIN;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let intentionalClose = false;

	const handlers = new Map<MessageKey, MessageHandler>();

	// Track active subscriptions for resubscribe on reconnect
	let activeSubs: ClientMessage[] = [];

	function getWsUrl(): string {
		const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
		return `${proto}//${location.host}/api/ws`;
	}

	function connect(): void {
		if (socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) {
			return;
		}

		intentionalClose = false;
		socket = new WebSocket(getWsUrl());

		socket.onopen = () => {
			connected = true;
			reconnectDelay = RECONNECT_MIN;

			// Resubscribe to active channels
			for (const msg of activeSubs) {
				sendRaw(msg);
			}
		};

		socket.onmessage = (event) => {
			const msg = JSON.parse(event.data) as ServerMessage;
			handleMessage(msg);
		};

		socket.onclose = () => {
			connected = false;
			socket = null;
			if (!intentionalClose) {
				scheduleReconnect();
			}
		};

		socket.onerror = () => {
			// onclose will fire after onerror
		};
	}

	function disconnect(): void {
		intentionalClose = true;
		activeSubs = [];
		if (reconnectTimer !== null) {
			clearTimeout(reconnectTimer);
			reconnectTimer = null;
		}
		socket?.close();
		socket = null;
		connected = false;
	}

	function scheduleReconnect(): void {
		if (reconnectTimer !== null) return;
		reconnectTimer = setTimeout(() => {
			reconnectTimer = null;
			connect();
		}, reconnectDelay);
		reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX);
	}

	function sendRaw(msg: ClientMessage): void {
		if (socket?.readyState === WebSocket.OPEN) {
			socket.send(JSON.stringify(msg));
		}
	}

	function handleMessage(msg: ServerMessage): void {
		if (msg.type === 'ping') {
			sendRaw({ type: 'pong' });
			return;
		}

		if (msg.type === 'error') {
			console.error('[ws] server error:', msg.message);
			return;
		}

		if ((msg.type === 'snapshot' || msg.type === 'delta') && msg.channel) {
			const key: MessageKey = `${msg.type}:${msg.channel}`;
			const handler = handlers.get(key);
			if (handler) {
				handler(msg.data);
			}
		}
	}

	function subscribe(
		channel: 'tasks' | 'planning',
		params: { view?: string; context?: string }
	): void {
		const msg: ClientMessage = {
			type: 'subscribe',
			channel,
			...params
		};

		// Replace existing sub for this channel
		activeSubs = activeSubs.filter(
			(s) => s.type !== 'subscribe' || (s as { channel: string }).channel !== channel
		);
		activeSubs.push(msg);

		sendRaw(msg);
	}

	function unsubscribe(channel: 'tasks' | 'planning'): void {
		activeSubs = activeSubs.filter(
			(s) => s.type !== 'subscribe' || (s as { channel: string }).channel !== channel
		);
		sendRaw({ type: 'unsubscribe', channel });
	}

	function onMessage(
		type: 'snapshot' | 'delta',
		channel: 'tasks' | 'planning',
		handler: MessageHandler
	): () => void {
		const key: MessageKey = `${type}:${channel}`;
		handlers.set(key, handler);
		return () => {
			handlers.delete(key);
		};
	}

	// Visibility handling: reconnect if disconnected when tab becomes visible
	if (typeof document !== 'undefined') {
		document.addEventListener('visibilitychange', () => {
			if (!document.hidden && !connected && !intentionalClose) {
				connect();
			}
		});
	}

	return {
		get connected() {
			return connected;
		},
		connect,
		disconnect,
		subscribe,
		unsubscribe,
		onMessage
	};
}

export const wsClient = createWSClient();

// Re-export data types for convenience
export type { SnapshotTasksData, DeltaTasksData, SnapshotPlanningData, DeltaPlanningData };
