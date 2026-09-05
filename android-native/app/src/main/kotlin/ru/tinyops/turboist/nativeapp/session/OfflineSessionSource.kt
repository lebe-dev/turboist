package ru.tinyops.turboist.nativeapp.session

import kotlinx.coroutines.flow.StateFlow

/**
 * Whether the signed-in session has actually been proved against the server.
 *
 * [SessionState.LoggedIn] deliberately does not answer this: a stored session
 * that could not be checked because the phone had no signal still opens the
 * app, because refusing to would deny the user data that is already on the
 * device. But the app is then showing a replica it has not been able to
 * reconcile, and that is worth saying out loud rather than pretending
 * everything is current.
 *
 * True only until the session is proved, which happens either when the device
 * reports a usable network again or on the first request that reaches the
 * server — whichever comes first. It is not a connectivity monitor: a phone can
 * be on a network that cannot reach this particular server, and this stays true
 * for exactly as long as that is the case.
 */
interface OfflineSessionSource {
    val unverifiedSession: StateFlow<Boolean>
}
