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
	| 'internal_error'
	| 'gone'
	// Federation trust-plane error codes (Federation v1 F0.3). These are
	// returned by the server-to-server federation endpoints and surfaced to the
	// browser only by the join flow (Phase 2); they are enumerated here so the
	// typed error mapping is complete as those milestones land.
	| 'federation_signature_invalid'
	| 'federation_replay'
	| 'federation_timestamp_stale'
	| 'federation_untrusted'
	| 'federation_key_missing'
	// Federation is not enabled on the target project (Federation v1 F1.2,
	// US-1.1 AC3). Surfaced when creating an invite on a non-federated project.
	| 'federation_not_enabled'
	| 'federation_digest_mismatch'
	// A handshake arrived from an instance_url already pinned to a DIFFERENT key
	// (Federation v1 F2.2, US-2.2 AC5). The join flow surfaces it as a key-mismatch
	// error (409); the owner refuses to silently rotate a pinned peer key.
	| 'federation_key_mismatch'
	// Federation protocol-version negotiation found no common version
	// (Federation v1 F0.4, US-9.1). Surfaced by the join flow as a
	// non-retryable error (Phase 2 / F6.1).
	| 'federation_version_unsupported'
	// A local mutation was attempted on a read-only joined federated project
	// (Federation v1 F2.4, US-2.4 AC4). The project page treats it as a graceful
	// read-only signal — revert the optimistic edit and toast, never crash.
	| 'federation_read_only'
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

export function isApiErrorEnvelope(value: unknown): value is ApiErrorEnvelope {
	if (typeof value !== 'object' || value === null) return false;
	const err = (value as { error?: unknown }).error;
	if (typeof err !== 'object' || err === null) return false;
	const e = err as { code?: unknown; message?: unknown };
	return typeof e.code === 'string' && typeof e.message === 'string';
}
