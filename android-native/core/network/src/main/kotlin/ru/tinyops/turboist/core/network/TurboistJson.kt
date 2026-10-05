package ru.tinyops.turboist.core.network

import kotlinx.serialization.json.Json

/**
 * The one JSON configuration the whole HTTP layer uses.
 *
 * - Unknown keys are ignored, so a server that grows a field does not break a
 *   client that was installed before it existed. The app is self-hosted: the
 *   phone and the server are upgraded on different days, always.
 * - Nulls are not written. Combined with nullable properties defaulting to
 *   `null`, that gives PATCH bodies their "absent means unchanged" behaviour for
 *   free; a field that must be *cleared* says so explicitly with [Clearable].
 * - Defaults are not written either, for the same reason: a PATCH body must
 *   carry the fields the user actually changed and nothing else.
 */
val TurboistJson: Json =
    Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        encodeDefaults = false
        coerceInputValues = false
    }
