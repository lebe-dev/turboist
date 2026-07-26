export type ApiErrorCode =
	| 'validation_failed'
	| 'auth_invalid'
	| 'auth_expired'
	| 'auth_rate_limited'
	| 'forbidden'
	| 'not_found'
	| 'conflict'
	| 'setup_already_done'
	| 'setup_required'
	| 'limit_exceeded'
	| 'forbidden_placement'
	| 'recurrence_invalid'
	| 'calendar_reauth_required'
	| 'internal_error'
	| 'network_error'
	| 'unknown_error';

export interface ApiErrorDetails {
	[key: string]: unknown;
}

export class ApiError extends Error {
	readonly code: ApiErrorCode | string;
	readonly status: number;
	readonly details: ApiErrorDetails | undefined;

	constructor(
		code: ApiErrorCode | string,
		message: string,
		status: number,
		details?: ApiErrorDetails
	) {
		super(message);
		this.name = 'ApiError';
		this.code = code;
		this.status = status;
		this.details = details;
	}
}

export interface ApiErrorEnvelope {
	error: {
		code: string;
		message: string;
		details?: ApiErrorDetails;
	};
}

/**
 * True when the failure is a Google Calendar re-authorization requirement —
 * the stored OAuth grant expired or was revoked, so the user must reconnect.
 * Backend signals this with a 409 `calendar_reauth_required` envelope.
 */
export function isCalendarReauthError(err: unknown): boolean {
	return err instanceof ApiError && err.code === 'calendar_reauth_required';
}

export function isApiErrorEnvelope(value: unknown): value is ApiErrorEnvelope {
	if (typeof value !== 'object' || value === null) return false;
	const err = (value as { error?: unknown }).error;
	if (typeof err !== 'object' || err === null) return false;
	const e = err as { code?: unknown; message?: unknown };
	return typeof e.code === 'string' && typeof e.message === 'string';
}
