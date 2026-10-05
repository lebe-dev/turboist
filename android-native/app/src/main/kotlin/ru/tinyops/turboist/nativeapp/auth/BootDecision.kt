package ru.tinyops.turboist.nativeapp.auth

/** What renewing the stored session at launch produced. */
enum class RenewOutcome {
    /** A fresh access token; the session is verified and current. */
    RENEWED,

    /**
     * The server could not be reached. This is not an authentication failure and
     * must never be treated as one: the stored token is very likely still valid,
     * and the user is simply somewhere without a signal.
     */
    UNREACHABLE,

    /** The server answered and refused the token. It is dead and has been forgotten. */
    REJECTED,

    /** Nothing was stored, so nothing was asked. A fresh install, or a signed-out one. */
    NO_TOKEN,
}

/** Which screen a launch ends on. */
enum class BootDecision {
    /** No server has been chosen yet. */
    Connect,

    /** The server has no account; the only way forward is creating it. */
    Setup,

    /** There is an account and no usable session. */
    SignIn,

    /** A verified session: the app opens. */
    App,

    /**
     * A stored session that could not be checked because the server was
     * unreachable. The app opens anyway, reading the replica, and the session is
     * verified the first time the network comes back.
     */
    OfflineApp,
}

/**
 * Decides what a launch does, from the three facts a launch can establish.
 *
 * This is the rule that makes the app usable on a train, and it is one line
 * away from the rule that makes it useless there: an unreachable server is not
 * a rejected credential. Sending a user with a perfectly good token to the
 * sign-in screen because their phone had no signal would also mean they cannot
 * sign in — the server is down — so they would lose access to data already on
 * the device. Hence [BootDecision.OfflineApp].
 *
 * The mirror of that rule is that a real rejection is acted on immediately: the
 * token is gone, so the session ends. The replica and the queue survive it, so
 * signing in again resumes exactly where the user was rather than starting from
 * an empty database.
 *
 * Kept a pure function so the whole matrix is covered by plain unit tests rather
 * than by launching an app against a server that has to be made to misbehave.
 */
fun bootDecision(
    serverConfigured: Boolean,
    renew: RenewOutcome,
    setup: SetupProbe,
): BootDecision {
    if (!serverConfigured) return BootDecision.Connect
    return when (renew) {
        RenewOutcome.RENEWED -> BootDecision.App
        RenewOutcome.UNREACHABLE -> BootDecision.OfflineApp
        RenewOutcome.REJECTED -> BootDecision.SignIn
        // Never signed in on this device. Which of the two entry screens applies
        // is the server's answer; when it did not give one, offer sign-in — the
        // attempt itself comes back saying setup is required, and that answer
        // moves the user on without a second guess here.
        RenewOutcome.NO_TOKEN -> if (setup == SetupProbe.REQUIRED) BootDecision.Setup else BootDecision.SignIn
    }
}
