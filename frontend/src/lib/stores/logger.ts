export interface LogEntry {
	id: number;
	timestamp: number;
	level: 'info' | 'warn' | 'error';
	tag: string;
	message: string;
}

const MAX_ENTRIES = 500;

const _entries: LogEntry[] = [];
let _version = 0;
let _nextId = 0;
const _listeners = new Set<() => void>();

function push(level: LogEntry['level'], tag: string, message: string): void {
	_entries.unshift({ id: _nextId++, timestamp: Date.now(), level, tag, message });
	if (_entries.length > MAX_ENTRIES) _entries.length = MAX_ENTRIES;
	_version++;
	for (const fn of _listeners) fn();
}

export const logger = {
	get version() {
		return _version;
	},
	get entries(): readonly LogEntry[] {
		return _entries;
	},
	log(tag: string, message: string): void {
		console.log(`[${tag}]`, message);
		push('info', tag, message);
	},
	warn(tag: string, message: string): void {
		console.warn(`[${tag}]`, message);
		push('warn', tag, message);
	},
	error(tag: string, message: string): void {
		console.error(`[${tag}]`, message);
		push('error', tag, message);
	},
	clear(): void {
		_entries.length = 0;
		_version++;
		for (const fn of _listeners) fn();
	},
	subscribe(fn: () => void): () => void {
		_listeners.add(fn);
		return () => _listeners.delete(fn);
	}
};
