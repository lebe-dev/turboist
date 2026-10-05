package ru.tinyops.turboist.core.network.api

import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path
import ru.tinyops.turboist.core.network.dto.PasskeyCeremonyDto
import ru.tinyops.turboist.core.network.dto.PasskeyDto
import ru.tinyops.turboist.core.network.dto.PasskeyLoginBeginRequest
import ru.tinyops.turboist.core.network.dto.PasskeyLoginFinishRequest
import ru.tinyops.turboist.core.network.dto.PasskeyRegisterFinishRequest
import ru.tinyops.turboist.core.network.dto.PasskeyRenameRequest
import ru.tinyops.turboist.core.network.dto.SessionDto

/**
 * Signing in with a passkey, and managing the ones the account has.
 *
 * Every ceremony is two calls around one platform prompt: `begin` hands out a
 * challenge the server holds for a few minutes, the device answers it, `finish`
 * verifies the answer. The challenge is single-use, so a `finish` that arrives
 * twice is refused rather than replayed.
 *
 * The two halves of this interface have different callers on purpose. The login
 * pair is unauthenticated — it is how a session is obtained. The management
 * calls run inside a signed-in session and are refused outright for an API
 * token: enrolling a credential grants a password-equivalent way in, and a
 * leaked token must not be able to bolt one onto the account.
 */
interface PasskeyApi {
    /**
     * Starts a usernameless login. The credential is resident on the device, so
     * there is nothing to say about who is signing in.
     */
    @POST("auth/passkey/login/begin")
    suspend fun loginBegin(
        @Body body: PasskeyLoginBeginRequest,
    ): PasskeyCeremonyDto

    /**
     * Verifies the assertion and answers with a session — a complete one, even
     * for an account that has a second factor configured.
     */
    @POST("auth/passkey/login/finish")
    suspend fun loginFinish(
        @Body body: PasskeyLoginFinishRequest,
    ): SessionDto

    /** The account's registered credentials, newest activity first. */
    @GET("api/v1/passkeys")
    suspend fun list(): List<PasskeyDto>

    /**
     * Creation options for a new credential. Credentials the account already has
     * come back in `excludeCredentials`, so the platform offers to replace one
     * rather than to stack a duplicate on the same device.
     */
    @POST("api/v1/passkeys/register/begin")
    suspend fun registerBegin(): PasskeyCeremonyDto

    /** Verifies the attestation and stores the credential. */
    @POST("api/v1/passkeys/register/finish")
    suspend fun registerFinish(
        @Body body: PasskeyRegisterFinishRequest,
    ): PasskeyDto

    /** Relabels a stored credential; the label is all this changes. */
    @PATCH("api/v1/passkeys/{id}")
    suspend fun rename(
        @Path("id") id: Long,
        @Body body: PasskeyRenameRequest,
    ): PasskeyDto

    /**
     * Removes a credential. Removing the last one is allowed: the password is the
     * recovery path and stays, so this can never lock the account out.
     */
    @DELETE("api/v1/passkeys/{id}")
    suspend fun remove(
        @Path("id") id: Long,
    ): Response<Unit>
}
