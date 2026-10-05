package ru.tinyops.turboist.core.network.dto

import kotlinx.serialization.Serializable

/**
 * One way in that is currently open: a browser, a phone or a script holding a
 * live session.
 *
 * Nothing here is replicated. The answer to "what can reach my account right
 * now" is only true at the instant the server gives it, and a copy kept on the
 * device would go on claiming a revoked session is alive — which is the one
 * mistake this list must not make.
 *
 * @property displayName what the server made of the user agent, so the client
 *   does not have to parse one.
 * @property ipAddress where the session was opened from. Captured once, at
 *   creation, and empty for sessions older than the field.
 * @property isCurrent the session this very request is being made with.
 */
@Serializable
data class ActiveSessionDto(
    val id: Long = 0,
    val clientKind: String = "",
    val userAgent: String = "",
    val displayName: String = "",
    val ipAddress: String = "",
    val createdAt: String = "",
    val lastUsedAt: String = "",
    val isCurrent: Boolean = false,
)

/**
 * A long-lived token an external tool authenticates with, as it can be listed:
 * metadata only.
 *
 * The secret itself is never in this shape. It exists in exactly one response —
 * the one that created it — and the server keeps only a hash afterwards, so a
 * token nobody wrote down is gone rather than recoverable.
 */
@Serializable
data class ApiTokenDto(
    val id: Long = 0,
    val name: String = "",
    val scopes: List<String> = emptyList(),
    val createdAt: String = "",
)

/**
 * Mints a token.
 *
 * The scopes are fixed for the life of the token: there is no endpoint that
 * widens or narrows one, and changing what a tool may do means deleting its
 * token and issuing another.
 */
@Serializable
data class CreateApiTokenRequest(
    val name: String,
    val scopes: List<String>,
)

/**
 * The one and only response carrying a token's plaintext.
 *
 * It is a separate type from [ApiTokenDto] so that "has a secret in it" is
 * visible in the type rather than in a nullable field somebody might log. The
 * value it carries must be shown to the user and then dropped: it is never
 * written to storage, and asking the server for it again is not possible.
 */
@Serializable
data class CreatedApiTokenDto(
    val id: Long = 0,
    val name: String = "",
    val scopes: List<String> = emptyList(),
    val token: String = "",
    val createdAt: String = "",
)

/**
 * What a pending second-factor enrolment offers the user: the shared secret, in
 * the three forms an authenticator app can take it.
 *
 * The secret is live from the moment this arrives — the server has stored it,
 * unconfirmed — so it is held in memory for as long as the enrolment screen is
 * up and never any longer.
 *
 * @property qrPngBase64 the same secret drawn as a QR image, so the phone does
 *   not have to render one itself.
 */
@Serializable
data class TotpEnrolmentDto(
    val secret: String = "",
    val otpauthUrl: String = "",
    val qrPngBase64: String = "",
)

/**
 * A six-digit code from the authenticator app, or — when switching the second
 * factor off — one of the recovery codes instead.
 */
@Serializable
data class TotpCodeRequest(
    val code: String,
)

/**
 * The single-use codes that get the account back when the authenticator is lost.
 *
 * Returned once, by the call that turns the second factor on, and never
 * obtainable again: the server keeps only hashes. The user has to record them
 * while they are on screen, which is why the screen says so before showing them.
 */
@Serializable
data class TotpRecoveryCodesDto(
    val recoveryCodes: List<String> = emptyList(),
)
