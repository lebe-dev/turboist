package ru.tinyops.turboist.core.network.api

import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import ru.tinyops.turboist.core.network.dto.ActiveSessionDto
import ru.tinyops.turboist.core.network.dto.ApiTokenDto
import ru.tinyops.turboist.core.network.dto.CreateApiTokenRequest
import ru.tinyops.turboist.core.network.dto.CreatedApiTokenDto
import ru.tinyops.turboist.core.network.dto.TotpCodeRequest
import ru.tinyops.turboist.core.network.dto.TotpEnrolmentDto
import ru.tinyops.turboist.core.network.dto.TotpRecoveryCodesDto

/**
 * Which sessions can currently reach the account, and how to end one.
 *
 * Refused outright for a long-lived API token, by the server: a leaked
 * integration credential must not be able to sign every browser and phone out of
 * the account it was issued for.
 */
interface SessionApi {
    /** The live sessions, most recently used first, with this one flagged. */
    @GET("api/v1/sessions")
    suspend fun list(): List<ActiveSessionDto>

    /**
     * Ends one session. The device holding it is signed out at once and has to
     * sign in again; an id that names no session of this account is refused
     * rather than silently ignored.
     */
    @DELETE("api/v1/sessions/{id}")
    suspend fun revoke(
        @Path("id") id: Long,
    ): Response<Unit>
}

/**
 * The long-lived tokens external tools authenticate with.
 *
 * Session-only, like the session list and for the same reason: a token that
 * could mint another token would make revoking one pointless.
 */
interface ApiTokenApi {
    /**
     * The tokens that exist, as metadata. The plaintext is not here and cannot
     * be — the server kept only a hash of it.
     */
    @GET("api/v1/api-tokens")
    suspend fun list(): List<ApiTokenDto>

    /**
     * Mints a token and answers with its plaintext, the only time it is ever
     * readable. The scopes are validated strictly and are immutable afterwards.
     */
    @POST("api/v1/api-tokens")
    suspend fun create(
        @Body body: CreateApiTokenRequest,
    ): CreatedApiTokenDto

    /** Revokes a token. Whatever was authenticating with it stops working at once. */
    @DELETE("api/v1/api-tokens/{id}")
    suspend fun delete(
        @Path("id") id: Long,
    ): Response<Unit>
}

/**
 * Turning the time-based second factor on and off.
 *
 * These routes exist only on an instance configured to offer a second factor at
 * all; on one that is not, they are simply absent and answer as unknown paths.
 * That refusal is the honest answer to "can this account have 2FA", so it is
 * read as such rather than treated as a failure.
 */
interface TotpApi {
    /**
     * Generates a fresh secret and stores it unconfirmed.
     *
     * Nothing is enabled by this call: an enrolment abandoned here leaves the
     * account exactly as it was, still signing in with a password alone.
     */
    @POST("auth/totp/setup")
    suspend fun setup(): TotpEnrolmentDto

    /**
     * Proves the authenticator holds the pending secret, and turns the second
     * factor on.
     *
     * The recovery codes come back with it, once. They are not stored anywhere on
     * the device — the user records them or does without them.
     */
    @POST("auth/totp/confirm")
    suspend fun confirm(
        @Body body: TotpCodeRequest,
    ): TotpRecoveryCodesDto

    /** Turns the second factor off. The code may be a current one or a recovery code. */
    @POST("auth/totp/disable")
    suspend fun disable(
        @Body body: TotpCodeRequest,
    ): Response<Unit>
}
