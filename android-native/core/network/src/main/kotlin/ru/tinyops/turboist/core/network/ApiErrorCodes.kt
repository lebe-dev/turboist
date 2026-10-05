package ru.tinyops.turboist.core.network

/**
 * The `code` values the server puts in an error envelope, plus the two this
 * client raises on its own behalf.
 *
 * The code — not the HTTP status — is what behaviour keys off: several distinct
 * conditions share a status (409 covers a capacity limit, a blocked task and a
 * replaced sync history) and the status alone cannot tell them apart.
 */
object ApiErrorCodes {
    const val VALIDATION_FAILED = "validation_failed"
    const val AUTH_INVALID = "auth_invalid"
    const val AUTH_EXPIRED = "auth_expired"
    const val AUTH_RATE_LIMITED = "auth_rate_limited"
    const val FORBIDDEN = "forbidden"
    const val NOT_FOUND = "not_found"
    const val CONFLICT = "conflict"
    const val SETUP_ALREADY_DONE = "setup_already_done"
    const val SETUP_REQUIRED = "setup_required"
    const val LIMIT_EXCEEDED = "limit_exceeded"
    const val FORBIDDEN_PLACEMENT = "forbidden_placement"
    const val RECURRENCE_INVALID = "recurrence_invalid"
    const val TROIKI_SLOT_FULL = "troiki_slot_full"
    const val TASK_BLOCKED = "task_blocked"
    const val TOTP_INVALID_CODE = "totp_invalid_code"
    const val TOTP_ALREADY_ENABLED = "totp_already_enabled"
    const val TOTP_NOT_ENABLED = "totp_not_enabled"
    const val CALENDAR_REAUTH_REQUIRED = "calendar_reauth_required"
    const val IDEMPOTENCY_IN_FLIGHT = "idempotency_in_flight"
    const val PASSKEY_CEREMONY_INVALID = "passkey_ceremony_invalid"
    const val PASSKEY_EXISTS = "passkey_exists"
    const val SYNC_EPOCH_MISMATCH = "sync_epoch_mismatch"
    const val SYNC_CURSOR_EXPIRED = "sync_cursor_expired"
    const val INTERNAL_ERROR = "internal_error"

    /** Raised locally when the request never reached a server. Never sent by one. */
    const val NETWORK_ERROR = "network_error"

    /** Raised locally when a response carried no envelope this client could read. */
    const val UNKNOWN_ERROR = "unknown_error"

    /**
     * Raised locally when the row a queued write was going to change no longer
     * exists: the server reported it deleted before the write could be sent. The
     * write can never land, so it leaves the queue and is kept where the user can
     * see what was lost. Never sent by a server.
     */
    const val TARGET_GONE = "target_gone"

    /**
     * Raised locally when a queued write cannot be turned into a request at all:
     * its payload was written by a build this one cannot read, or it names a row
     * whose own creation never reached the server. Either way no amount of
     * retrying changes the answer, so the write is kept where the user can see
     * it rather than retried forever. Never sent by a server.
     */
    const val WRITE_UNSENDABLE = "write_unsendable"
}

/** Header names this client sets or reads on top of the standard ones. */
object ApiHeaders {
    const val AUTHORIZATION = "Authorization"

    /** Makes a mutation safe to retry: the server replays the stored response instead of re-running it. */
    const val IDEMPOTENCY_KEY = "Idempotency-Key"

    /** Set by the server on a response it replayed rather than produced. */
    const val IDEMPOTENT_REPLAY = "X-Idempotent-Replay"
}
