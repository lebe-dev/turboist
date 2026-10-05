package ru.tinyops.turboist.nativeapp.session

/** Which of the three top-level content trees the shell renders. */
enum class ShellGraph {
    /** A neutral splash while the stored session is being resolved. */
    Splash,

    /** The connect / setup / sign-in screens. */
    Auth,

    /** The task views, behind the navigation drawer. */
    App,
}

/**
 * Maps session state onto the content tree to render.
 *
 * Kept as a pure function rather than a `when` inlined into a composable so the
 * rule that decides whether a user sees their tasks is covered by a plain unit
 * test instead of a UI test.
 */
fun graphFor(state: SessionState): ShellGraph =
    when (state) {
        SessionState.Connecting -> ShellGraph.Splash
        SessionState.LoggedOut -> ShellGraph.Auth
        SessionState.LoggedIn -> ShellGraph.App
    }
