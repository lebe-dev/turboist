package ru.tinyops.turboist.core.model

/**
 * Per-user interface state that follows the user across devices, as opposed to
 * the preferences in [UserSettings].
 *
 * The server keeps this as an opaque JSON object and merges writes key by key, so
 * modelling only the keys this client uses is safe: a key it does not know is
 * left untouched on the server rather than overwritten.
 *
 * @property activeContextId the context the workspace is filtered to, or `null`
 *   for all of them. A **server** context id, because that is what the blob
 *   carries in both directions.
 */
data class UserState(
    val activeContextId: Long? = null,
)
