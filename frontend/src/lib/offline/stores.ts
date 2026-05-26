import type { EntityKind } from './db';

const EVT_PREFIX = 'turboist:db-changed:' as const;
const OUTBOX_EVT = 'turboist:outbox-changed';

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

export const emitOutboxChanged = (): void => {
	target.dispatchEvent(new CustomEvent(OUTBOX_EVT));
};

export const onOutboxChanged = (handler: () => void): (() => void) => {
	target.addEventListener(OUTBOX_EVT, handler);
	return () => target.removeEventListener(OUTBOX_EVT, handler);
};
