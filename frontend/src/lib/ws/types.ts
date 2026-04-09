import type { Meta, Task, TroikiState, TroikiSectionState } from '$lib/api/types';

export type Channel = 'tasks' | 'planning' | 'troiki';

// Client → Server
export interface SubscribeMessage {
	type: 'subscribe';
	channel: Channel;
	view?: string;
	context?: string;
	seq?: number;
}

export interface UnsubscribeMessage {
	type: 'unsubscribe';
	channel: Channel;
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

export type SnapshotTroikiData = TroikiState;

export interface DeltaTroikiData {
	sections: TroikiSectionState[];
}

export interface ServerMessage {
	type: 'snapshot' | 'delta' | 'ping' | 'error';
	channel?: Channel;
	data?: unknown;
	message?: string;
	seq?: number;
}
