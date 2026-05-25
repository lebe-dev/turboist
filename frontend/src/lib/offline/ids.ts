import { ulid } from 'ulid';

export const newClientId = (): string => ulid();

export const newOutboxId = (): string => ulid();

export type Ref = { kind: 'ref'; clientId: string };

export const ref = (clientId: string): Ref => ({ kind: 'ref', clientId });

export const isRef = (value: unknown): value is Ref =>
	typeof value === 'object' &&
	value !== null &&
	(value as { kind?: unknown }).kind === 'ref' &&
	typeof (value as { clientId?: unknown }).clientId === 'string';
