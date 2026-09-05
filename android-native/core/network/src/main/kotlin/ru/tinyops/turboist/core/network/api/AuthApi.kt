package ru.tinyops.turboist.core.network.api

import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import ru.tinyops.turboist.core.network.dto.LoginRequest
import ru.tinyops.turboist.core.network.dto.LoginResponseDto
import ru.tinyops.turboist.core.network.dto.MeDto
import ru.tinyops.turboist.core.network.dto.OtpLoginRequest
import ru.tinyops.turboist.core.network.dto.PublicConfigDto
import ru.tinyops.turboist.core.network.dto.RefreshRequest
import ru.tinyops.turboist.core.network.dto.SessionDto
import ru.tinyops.turboist.core.network.dto.SetupRequest

/**
 * Sign-in and session lifetime.
 *
 * These endpoints live outside the versioned API and are the only ones a client
 * may call before it has a session. They are also the only ones exempt from the
 * automatic bearer token and the refresh-and-retry, which is what keeps a failed
 * refresh from trying to refresh itself.
 */
interface AuthApi {
    /**
     * The unauthenticated probe that proves an address really is a Turboist
     * server, before any credentials are typed into it.
     */
    @GET("api/config")
    suspend fun publicConfig(): PublicConfigDto

    /**
     * Asks whether the instance still needs its first account.
     *
     * There is no endpoint for the question: the setup gate sits in front of the
     * whole versioned API and short-circuits every call to it with
     * `setup_required` until an account exists. So the cheapest versioned read is
     * made without credentials and only its refusal is read — `setup_required`
     * means "create the account", anything else means "sign in". Nothing in the
     * body is used, which is why it is typed as an empty response.
     */
    @GET("api/v1/config")
    suspend fun setupProbe(): Response<Unit>

    /**
     * Creates the single account this installation has. Succeeds exactly once;
     * every later attempt is refused.
     */
    @POST("auth/setup")
    suspend fun setup(
        @Body body: SetupRequest,
    ): SessionDto

    /**
     * Signs in. Answers with a session, or — when the account has a second factor
     * — with a challenge, both as a 200. Read the result through `toOutcome()`
     * rather than by inspecting the fields.
     */
    @POST("auth/login")
    suspend fun login(
        @Body body: LoginRequest,
    ): LoginResponseDto

    /** Finishes a two-step sign-in with a time-based code or a recovery code. */
    @POST("auth/login/otp")
    suspend fun loginWithOtp(
        @Body body: OtpLoginRequest,
    ): SessionDto

    /**
     * Rotates the session. The old refresh token dies with the call: presenting it
     * twice is how the server detects a stolen one, and it revokes the session.
     */
    @POST("auth/refresh")
    suspend fun refresh(
        @Body body: RefreshRequest,
    ): SessionDto

    /** Ends this session only. Other devices stay signed in. */
    @POST("auth/logout")
    suspend fun logout(): Response<Unit>

    /** Ends every session, this one included. */
    @POST("auth/logout-all")
    suspend fun logoutAll(): Response<Unit>

    /** Ends every session except this one — for kicking a device left behind. */
    @POST("auth/logout-others")
    suspend fun logoutOthers(): Response<Unit>

    @GET("auth/me")
    suspend fun me(): MeDto
}
