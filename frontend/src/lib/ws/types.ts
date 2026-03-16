import type { Meta, Task } from '$lib/api/types';

// Client → Server
export interface SubscribeMessage {
	type: 'subscribe';
	channel: 'tasks' | 'planning';
	view?: string;
	context?: string;
}

export interface UnsubscribeMessage {
	type: 'unsubscribe';
	channel: 'tasks' | 'planning';
}

export interface PongMessage {
	type: 'pong';
}

export type ClientMessage = SubscribeMessage | UnsubscribeMessage | PongMessage;

// Server → Client
export interface SnapshotTasksData {
	tasks: Task[];
	meta: Meta;
}

export interface DeltaTasksData {
	upserted: Task[];
	removed: string[];
	meta: Meta;
}

export interface SnapshotPlanningData {
	backlog: Task[];
	weekly: Task[];
	meta: Meta;
}

export interface DeltaPlanningData {
	backlog_upserted: Task[] | null;
	backlog_removed: string[] | null;
	weekly_upserted: Task[] | null;
	weekly_removed: string[] | null;
	meta: Meta;
}

export interface ServerMessage {
	type: 'snapshot' | 'delta' | 'ping' | 'error';
	channel?: 'tasks' | 'planning';
	data?: unknown;
	message?: string;
}
