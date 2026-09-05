package ru.tinyops.turboist.nativeapp.auth.passkey

/** What a passkey sign-in produced. */
sealed interface PasskeyOutcome {
    /** The session exists now; the shell switches to the app on its own. */
    data object SignedIn : PasskeyOutcome

    data class Failed(val problem: PasskeyProblem) : PasskeyOutcome
}
