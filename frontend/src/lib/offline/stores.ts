import type { EntityKind } from './db';

const EVT_PREFIX = 'turboist:db-changed:' as const;

const target: EventTarget = new EventTarget();

export const dbChangedEventName = (kind: EntityKind): string => `${EVT_PREFIX}${kind}`;

export const emitDbChanged = (kind: EntityKind): void => {
	target.dispatchEvent(new CustomEvent(dbChangedEventName(kind)));
};

export const onDbChanged = (kind: EntityKind, handler: () => void): (() => void) => {
	const name = dbChangedEventName(kind);
	target.addEventListener(name, handler);
	return () => target.removeEventListener(name, handler);
};
