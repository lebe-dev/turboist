export interface ApiLogEntry {
	id: number;
	timestamp: Date;
	method: string;
	url: string;
	status: number | null;
	durationMs: number;
	error: string | null;
	requestBody: string | null;
	responseBody: string | null;
}

const MAX_ENTRIES = 200;
let enabled = $state(false);
let entries = $state<ApiLogEntry[]>([]);
let nextId = 0;

export function addApiLogEntry(entry: Omit<ApiLogEntry, 'id'>): void {
	if (!enabled) return;
	entries = [{ ...entry, id: nextId++ }, ...entries].slice(0, MAX_ENTRIES);
}

export const apiLogStore = {
	get enabled() {
		return enabled;
	},
	get entries() {
		return entries;
	},
	setEnabled(v: boolean) {
		enabled = v;
		if (!v) {
			entries = [];
			nextId = 0;
		}
	},
	clear() {
		entries = [];
		nextId = 0;
	}
};
