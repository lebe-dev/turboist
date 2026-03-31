import { logger } from '$lib/stores/logger';
import type {
	ClientMessage,
	SubscribeMessage,
	ServerMessage,
	SnapshotTasksData,
	DeltaTasksData,
	SnapshotPlanningData,
	DeltaPlanningData
} from './types';

const RECONNECT_MIN = 1000;
const RECONNECT_MAX = 30000;

type MessageKey = `${'snapshot' | 'delta'}:${'tasks' | 'planning'}`;
type MessageHandler = (data: unknown, seq?: number) => void;

function createWSClient() {
	let connected = $state(false);

	let socket: WebSocket | null = null;
	let reconnectDelay = RECONNECT_MIN;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let intentionalClose = false;

	const handlers = new Map<MessageKey, MessageHandler>();
	const stateListeners = new Set<(connected: boolean) => void>();

	// Track active subscriptions for resubscribe on reconnect
	let activeSubs: SubscribeMessage[] = [];

	// Monotonically increasing subscription sequence number
	let subSeq = 0;

	function getWsUrl(): string {
		const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
		return `${proto}//${location.host}/api/ws`;
	}

	function connect(): void {
		if (socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) {
			return;
		}

		intentionalClose = false;
		const url = getWsUrl();
		logger.log('ws', `connecting to ${url}`);
		socket = new WebSocket(url);

		socket.onopen = () => {
			connected = true;
			reconnectDelay = RECONNECT_MIN;
			logger.log('ws', 'connected');

			// Notify listeners
			for (const fn of stateListeners) fn(true);

			// Resubscribe to active channels with fresh seq
			if (activeSubs.length > 0) {
				subSeq++;
				for (const msg of activeSubs) {
					logger.log('ws', `resubscribe ${(msg as { channel?: string }).channel}`);
					sendRaw({ ...msg, seq: subSeq });
				}
			}
		};

		socket.onmessage = (event) => {
			const msg = JSON.parse(event.data) as ServerMessage;
			handleMessage(msg);
		};

		socket.onclose = (event) => {
			const wasConnected = connected;
			connected = false;
			socket = null;
			logger.log('ws', `disconnected code=${event.code} reason=${event.reason} wasConnected=${wasConnected}`);

			// Notify listeners
			for (const fn of stateListeners) fn(false);

			if (!intentionalClose) {
				scheduleReconnect();
			}
		};

		socket.onerror = () => {
			logger.warn('ws', 'connection error');
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
		logger.log('ws', `reconnecting in ${reconnectDelay}ms`);
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
			logger.error('ws', `server error: ${msg.message}`);
			return;
		}

		if ((msg.type === 'snapshot' || msg.type === 'delta') && msg.channel) {
			logger.log('ws', `${msg.type} ${msg.channel} seq=${msg.seq}`);
			const key: MessageKey = `${msg.type}:${msg.channel}`;
			const handler = handlers.get(key);
			if (handler) {
				handler(msg.data, msg.seq);
			} else {
				logger.warn('ws', `no handler for ${key}`);
			}
		}
	}

	function subscribe(
		channel: 'tasks' | 'planning',
		params: { view?: string; context?: string }
	): void {
		subSeq++;
		const msg: SubscribeMessage = {
			type: 'subscribe',
			channel,
			...params
		};

		// Replace existing sub for this channel (stored without seq — seq is assigned at send time)
		activeSubs = activeSubs.filter((s) => s.channel !== channel);
		activeSubs.push(msg);

		const sent = socket?.readyState === WebSocket.OPEN;
		logger.log('ws', `subscribe ${channel} ${JSON.stringify(params)} seq=${subSeq} sent=${sent} readyState=${socket?.readyState}`);
		sendRaw({ ...msg, seq: subSeq });
	}

	function unsubscribe(channel: 'tasks' | 'planning'): void {
		activeSubs = activeSubs.filter((s) => s.channel !== channel);
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

	function onStateChange(handler: (connected: boolean) => void): () => void {
		stateListeners.add(handler);
		return () => {
			stateListeners.delete(handler);
		};
	}

	return {
		get connected() {
			return connected;
		},
		get currentSeq() {
			return subSeq;
		},
		connect,
		disconnect,
		subscribe,
		unsubscribe,
		onMessage,
		onStateChange
	};
}

export const wsClient = createWSClient();

// Re-export data types for convenience
export type { SnapshotTasksData, DeltaTasksData, SnapshotPlanningData, DeltaPlanningData };
