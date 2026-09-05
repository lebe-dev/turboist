package ru.tinyops.turboist.core.network.http

/**
 * How much of a failed response is read before it is parsed as an error
 * envelope. Errors are a few hundred bytes; the cap is what keeps a server that
 * answers a failure with a megabyte of HTML from being read into memory.
 */
internal const val MAX_ERROR_BODY_BYTES: Long = 64L * 1024L

/**
 * The path segment that marks the versioned API, the only part of the server that
 * honours an idempotency key. Matched anywhere in the path rather than at its
 * start, because a server can be mounted under a sub-path by a reverse proxy.
 */
internal const val VERSIONED_API_PREFIX: String = "/api/v1/"

/** The methods that change server state, and therefore the ones worth making replayable. */
internal val MUTATING_METHODS: Set<String> = setOf("POST", "PUT", "PATCH", "DELETE")
