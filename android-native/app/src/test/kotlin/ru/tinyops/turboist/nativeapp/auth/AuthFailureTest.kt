package ru.tinyops.turboist.nativeapp.auth

import ru.tinyops.turboist.core.network.ApiErrorBody
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException
import java.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * What each refusal is shown as.
 *
 * The distinction that earns its keep is "we could not reach the server" versus
 * "the server said no": telling a user their password is wrong when their phone
 * has no signal sends them off to reset a password that was fine.
 */
class AuthFailureTest {
    private fun refusal(
        status: Int,
        code: String,
    ) = ApiException.from(status, ApiErrorBody(code = code, message = "refused"))

    @Test
    fun `an unreachable server is not a wrong password`() {
        assertEquals(AuthFailure.Unreachable, authFailureOf(ApiException.Network("no route")))
    }

    @Test
    fun `a refused credential is reported as one`() {
        assertEquals(AuthFailure.BadCredentials, authFailureOf(refusal(401, ApiErrorCodes.AUTH_INVALID)))
    }

    @Test
    fun `a wrong second-factor code is told apart from a wrong password`() {
        assertEquals(AuthFailure.InvalidCode, authFailureOf(refusal(401, ApiErrorCodes.TOTP_INVALID_CODE)))
    }

    @Test
    fun `a throttled address is told to wait rather than to try other credentials`() {
        assertEquals(AuthFailure.TooManyAttempts, authFailureOf(refusal(429, ApiErrorCodes.AUTH_RATE_LIMITED)))
    }

    @Test
    fun `a second attempt at setup says the account already exists`() {
        assertEquals(AuthFailure.AlreadySetUp, authFailureOf(refusal(409, ApiErrorCodes.SETUP_ALREADY_DONE)))
    }

    @Test
    fun `values the server refused are reported as values, not as credentials`() {
        assertEquals(AuthFailure.InvalidDetails, authFailureOf(refusal(400, ApiErrorCodes.VALIDATION_FAILED)))
    }

    @Test
    fun `anything unrecognised still leaves a screen the user can retry from`() {
        assertEquals(AuthFailure.ServerError, authFailureOf(refusal(502, ApiErrorCodes.INTERNAL_ERROR)))
        assertEquals(AuthFailure.ServerError, authFailureOf(IOException("something else entirely")))
    }
}
