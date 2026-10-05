package ru.tinyops.turboist.nativeapp.auth

import okhttp3.HttpUrl
import ru.tinyops.turboist.core.network.ServerUrl

/** What reading a typed-in server address produced. */
sealed interface ServerAddress {
    /** A usable address, in the canonical form the HTTP stack expects. */
    data class Accepted(val url: HttpUrl) : ServerAddress

    /** The text does not name a host at all. */
    data object Unreadable : ServerAddress

    /** The text names a host over plain HTTP, which this build will not send credentials over. */
    data object NotEncrypted : ServerAddress
}

/**
 * Whether this installation is allowed to talk to a server over plain HTTP.
 *
 * There is exactly one switch, and it is the manifest: the platform permits
 * cleartext only where a build asked for it, and only the debug build does, so a
 * developer can point the app at a local server that terminates plain HTTP. The
 * shipping build cannot be talked into it by any code path, because there is no
 * second flag to get out of step with the first.
 */
class CleartextPolicy(val allowed: Boolean)

/**
 * Reads what the user typed on the connect screen.
 *
 * **Plain HTTP is refused unless the build permits it.** The address entered
 * here is the one the account password and every later session token travel to,
 * and a build that has not opted in cannot send cleartext at all — so an
 * `http://` address would not merely be unsafe, it would fail later with a
 * transport error that says nothing about why. Refusing it here turns that into
 * a sentence the user can act on. [allowCleartext] follows the platform's own
 * answer rather than a flag of its own, so this rule and what the transport will
 * actually send can never disagree.
 *
 * A bare host is read as `https://host` rather than guessed at, whatever the
 * policy: a typo must never silently downgrade the connection.
 */
fun readServerAddress(
    raw: String,
    allowCleartext: Boolean = false,
): ServerAddress {
    val normalized = ServerUrl.normalize(raw) ?: return ServerAddress.Unreadable
    if (normalized.scheme != "https" && !allowCleartext) return ServerAddress.NotEncrypted
    return ServerAddress.Accepted(normalized)
}
