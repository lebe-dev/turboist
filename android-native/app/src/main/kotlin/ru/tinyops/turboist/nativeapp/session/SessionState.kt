package ru.tinyops.turboist.nativeapp.session

import kotlinx.coroutines.flow.StateFlow

/**
 * Where the process is in the "which screens may the user see" question.
 *
 * Deliberately three states and no more. Anything finer — which server, whether
 * a second factor is pending, whether the last refresh failed because the
 * network was down rather than because the token was rejected — belongs to the
 * authentication implementation, not to the shell: the shell only has to pick
 * between the authentication screens and the app.
 */
sealed interface SessionState {
    /** Stored credentials have not been read yet; show a neutral splash. */
    data object Connecting : SessionState

    /** No usable credentials. The authentication flow owns the screen. */
    data object LoggedOut : SessionState

    /**
     * Credentials are usable. Note that "usable" is not "verified just now": a
     * stored session with the network down still renders the app, because a
     * network failure is not an authentication failure.
     */
    data object LoggedIn : SessionState
}

/**
 * The single seam between authentication and the rest of the app.
 *
 * The shell observes this and nothing else, so the authentication
 * implementation can be replaced without touching a single screen.
 */
interface SessionStateSource {
    val state: StateFlow<SessionState>
}
