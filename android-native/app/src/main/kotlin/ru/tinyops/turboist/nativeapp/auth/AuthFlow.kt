package ru.tinyops.turboist.nativeapp.auth

import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException

/** Which of the three screens the sign-in flow is on. */
enum class AuthStep {
    /** Choosing a server. */
    Connect,

    /** Creating the single account the server hosts. */
    Setup,

    /** Signing in to an account that already exists. */
    SignIn,
}

/**
 * Why an attempt did not go through, in the terms the user needs.
 *
 * Not the server's error code and not its message: the codes are an API
 * contract and the messages are written in one language, while what a screen
 * has to show is a translated sentence about what to do next. Several distinct
 * codes collapse onto one entry here precisely because the user's next move is
 * the same for all of them.
 */
enum class AuthFailure {
    /** The typed address does not name a server. */
    Unreadable,

    /** The typed address is plain HTTP, which this build refuses to send credentials over. */
    NotEncrypted,

    /** Nothing answered at that address, or the session could not be checked. */
    Unreachable,

    /** The username or password is wrong. */
    BadCredentials,

    /** The second-factor code is wrong or has expired. */
    InvalidCode,

    /** The server is refusing further attempts from this address for a while. */
    TooManyAttempts,

    /** The two password fields do not agree. */
    PasswordsDoNotMatch,

    /** The account already exists, so this server needs a sign-in rather than a setup. */
    AlreadySetUp,

    /** The server read the request and refused the values in it — too short a password, say. */
    InvalidDetails,

    /** The server answered, but with a failure of its own. */
    ServerError,
}

/** What an attempt produced. */
sealed interface AuthResult {
    /** The session exists now; the shell switches to the app on its own. */
    data object Done : AuthResult

    /** The account has a second factor and the flow needs the code. */
    data object OtpRequired : AuthResult

    /** The flow moved on to another screen without a session — connecting does this. */
    data class MovedTo(val step: AuthStep) : AuthResult

    data class Failed(val failure: AuthFailure) : AuthResult
}

/**
 * Reads a failed call as the sentence to show.
 *
 * Anything that is not a recognised refusal is [AuthFailure.ServerError] rather
 * than a crash: a self-hosted server can be behind a proxy that answers with
 * something this client has never seen, and that must still leave the user on a
 * screen they can retry from.
 */
fun authFailureOf(error: Throwable): AuthFailure =
    when {
        error is ApiException.Network -> AuthFailure.Unreachable
        error is ApiException.RateLimited -> AuthFailure.TooManyAttempts
        error is ApiException && error.code == ApiErrorCodes.TOTP_INVALID_CODE -> AuthFailure.InvalidCode
        error is ApiException && error.code == ApiErrorCodes.SETUP_ALREADY_DONE -> AuthFailure.AlreadySetUp
        error is ApiException && error.code == ApiErrorCodes.VALIDATION_FAILED -> AuthFailure.InvalidDetails
        error is ApiException.Auth -> AuthFailure.BadCredentials
        else -> AuthFailure.ServerError
    }
