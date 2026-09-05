package ru.tinyops.turboist.core.network.dto

import kotlinx.serialization.EncodeDefault
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.Serializable
import ru.tinyops.turboist.core.model.ClientKind

@Serializable
data class UserDto(
    val id: Long = 0,
    val username: String = "",
    val totpEnabled: Boolean = false,
)

/** First-run account creation. Succeeds exactly once per installation. */
@Serializable
data class SetupRequest(
    val username: String,
    val password: String,
    /**
     * Which client the session being opened belongs to. The server caps sessions per
     * kind and refuses a sign-in that does not name one.
     *
     * It is written to the wire even though it never changes, which is why it is
     * marked rather than left to the layer's usual rule. That rule omits any property
     * still holding its default so a PATCH body carries only what the user actually
     * changed — and a constant is indistinguishable from an untouched default, so it
     * would be dropped and the request refused.
     */
    @EncodeDefault(EncodeDefault.Mode.ALWAYS)
    @OptIn(ExperimentalSerializationApi::class)
    val clientKind: String = ClientKind.ANDROID.wire,
)

@Serializable
data class LoginRequest(
    val username: String,
    val password: String,
    /** Written to the wire always, for the reason given on [SetupRequest.clientKind]. */
    @EncodeDefault(EncodeDefault.Mode.ALWAYS)
    @OptIn(ExperimentalSerializationApi::class)
    val clientKind: String = ClientKind.ANDROID.wire,
)

/** Completes a two-step login with a time-based code or one of the recovery codes. */
@Serializable
data class OtpLoginRequest(
    val ticket: String,
    val code: String,
)

/**
 * Exchanges the rotating refresh token for a new pair.
 *
 * This client always sends the token in the body: the cookie form is for
 * browsers, and a native app has nowhere to put a cookie that is safer than the
 * encrypted storage the token already lives in.
 */
@Serializable
data class RefreshRequest(
    val refresh: String,
)

/**
 * What a login attempt answers with — either a session or a demand for the second
 * factor, both with a 200.
 *
 * The two shapes share one response type because that is what the endpoint does;
 * [LoginOutcome] is the thing worth branching on and is what callers should use.
 */
@Serializable
data class LoginResponseDto(
    val access: String? = null,
    val refresh: String? = null,
    val user: UserDto? = null,
    val otpRequired: Boolean = false,
    val ticket: String? = null,
)

/** A completed session: an access token, the rotating refresh token, and who it belongs to. */
@Serializable
data class SessionDto(
    val access: String = "",
    val refresh: String = "",
    val user: UserDto = UserDto(),
)

@Serializable
data class MeDto(
    val user: UserDto = UserDto(),
)

/**
 * The two ways a login ends. Reading this instead of the raw response is what
 * keeps "did it ask for a code?" from being a null check on two unrelated fields.
 */
sealed interface LoginOutcome {
    /** Signed in. The session is ready to use. */
    data class Session(val session: SessionDto) : LoginOutcome

    /**
     * The account has a second factor. The ticket is short-lived, bound to this
     * client kind, and usable only to finish this one login — so it is held in
     * memory and never written down.
     */
    data class OtpRequired(val ticket: String) : LoginOutcome
}

/**
 * Reads a login response as the outcome it actually is. A response claiming
 * neither a session nor a challenge is a server this client cannot talk to, and
 * saying so here beats a null-pointer failure three screens later.
 */
fun LoginResponseDto.toOutcome(): LoginOutcome {
    if (otpRequired) {
        val ticket = ticket
        require(!ticket.isNullOrEmpty()) { "the server asked for a second factor without a ticket" }
        return LoginOutcome.OtpRequired(ticket)
    }
    val access = access
    require(!access.isNullOrEmpty()) { "the server answered a login with neither a session nor a challenge" }
    return LoginOutcome.Session(SessionDto(access = access, refresh = refresh.orEmpty(), user = user ?: UserDto()))
}

/**
 * The unauthenticated probe the connect screen validates a server address with.
 * Reaching it proves the address really is a Turboist instance before any
 * credentials are typed into it.
 */
@Serializable
data class PublicConfigDto(
    val sentry: SentryConfigDto = SentryConfigDto(),
    val passkeys: PasskeyConfigDto = PasskeyConfigDto(),
)

@Serializable
data class SentryConfigDto(
    val dsn: String = "",
    val environment: String = "",
)

/**
 * @property enabled whether the instance brought WebAuthn up at all.
 * @property available whether at least one passkey is registered, which is what
 *   decides between offering passkey sign-in and offering nothing.
 */
@Serializable
data class PasskeyConfigDto(
    val enabled: Boolean = false,
    val available: Boolean = false,
)
