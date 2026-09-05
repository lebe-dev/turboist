package ru.tinyops.turboist.nativeapp.auth.passkey

import androidx.credentials.exceptions.CreateCredentialCancellationException
import androidx.credentials.exceptions.CreateCredentialNoCreateOptionException
import androidx.credentials.exceptions.CreateCredentialUnknownException
import androidx.credentials.exceptions.CreateCredentialUnsupportedException
import androidx.credentials.exceptions.GetCredentialCancellationException
import androidx.credentials.exceptions.GetCredentialProviderConfigurationException
import androidx.credentials.exceptions.GetCredentialUnsupportedException
import androidx.credentials.exceptions.NoCredentialException
import androidx.credentials.exceptions.domerrors.NotAllowedError
import androidx.credentials.exceptions.domerrors.SecurityError
import androidx.credentials.exceptions.publickeycredential.CreatePublicKeyCredentialDomException
import androidx.credentials.exceptions.publickeycredential.GetPublicKeyCredentialDomException
import ru.tinyops.turboist.core.network.ApiErrorBody
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * Reading a failed ceremony as the sentence the user needs.
 *
 * Everything a passkey can do wrong ends here, and the classification is what
 * decides whether the user is told to try again, to use their password, or that
 * the server's setup is incomplete. Getting it wrong is not a cosmetic problem:
 * a user told only "it failed" taps the same button again, on a device that will
 * refuse it again for the same reason.
 */
class PasskeyProblemTest {
    @Test
    fun `a dismissed sheet is a choice, not a failure`() {
        assertEquals(PasskeyProblem.Cancelled, passkeyProblemOf(GetCredentialCancellationException()))
        assertEquals(PasskeyProblem.Cancelled, passkeyProblemOf(CreateCredentialCancellationException()))
    }

    @Test
    fun `a device holding nothing for this account is told apart from a broken one`() {
        assertEquals(PasskeyProblem.NotEnrolled, passkeyProblemOf(NoCredentialException()))
        assertEquals(PasskeyProblem.NotEnrolled, passkeyProblemOf(CreateCredentialNoCreateOptionException()))
    }

    @Test
    fun `a device with no passkey support at all says so`() {
        assertEquals(PasskeyProblem.Unsupported, passkeyProblemOf(GetCredentialUnsupportedException()))
        assertEquals(PasskeyProblem.Unsupported, passkeyProblemOf(CreateCredentialUnsupportedException()))
        assertEquals(PasskeyProblem.Unsupported, passkeyProblemOf(GetCredentialProviderConfigurationException()))
    }

    @Test
    fun `a relying party the device will not accept is a setup problem, not a wrong credential`() {
        // The association the server publishes does not name this app, or names a
        // different signing certificate — the one failure the user cannot fix by
        // retrying, and the one the server owner has to hear about.
        assertEquals(
            PasskeyProblem.Untrusted,
            passkeyProblemOf(GetPublicKeyCredentialDomException(SecurityError(), "rp id not verified")),
        )
        assertEquals(
            PasskeyProblem.Untrusted,
            passkeyProblemOf(CreatePublicKeyCredentialDomException(SecurityError(), "rp id not verified")),
        )
    }

    @Test
    fun `any other ceremony failure still points at the password`() {
        assertEquals(
            PasskeyProblem.Failed,
            passkeyProblemOf(GetPublicKeyCredentialDomException(NotAllowedError(), "not allowed")),
        )
        assertEquals(PasskeyProblem.Failed, passkeyProblemOf(CreateCredentialUnknownException()))
        assertEquals(PasskeyProblem.Failed, passkeyProblemOf(IllegalStateException("something else")))
    }

    @Test
    fun `a server refusal is never read as a mistyped credential`() {
        // Nothing was typed, so there is nothing for the user to correct: the
        // useful answer is that this passkey did not work and the password does.
        assertEquals(PasskeyProblem.Refused, passkeyProblemOf(refusal(401, ApiErrorCodes.AUTH_INVALID)))
        assertEquals(
            PasskeyProblem.Refused,
            passkeyProblemOf(refusal(400, ApiErrorCodes.PASSKEY_CEREMONY_INVALID)),
        )
    }

    @Test
    fun `enrolment refusals keep their own wording`() {
        assertEquals(PasskeyProblem.AlreadyRegistered, passkeyProblemOf(refusal(409, ApiErrorCodes.PASSKEY_EXISTS)))
        assertEquals(PasskeyProblem.LimitReached, passkeyProblemOf(refusal(422, ApiErrorCodes.LIMIT_EXCEEDED)))
    }

    @Test
    fun `an instance with the feature switched off is reported as one, not as a broken device`() {
        // A server that never brought WebAuthn up does not route these calls at
        // all, so the client learns about it as a missing endpoint.
        assertEquals(PasskeyProblem.Unsupported, passkeyProblemOf(refusal(404, ApiErrorCodes.NOT_FOUND)))
    }

    @Test
    fun `a server out of reach is a network problem, not a passkey problem`() {
        assertEquals(
            PasskeyProblem.Unreachable,
            passkeyProblemOf(ApiException.Network("the request did not reach the server")),
        )
    }

    private fun refusal(
        status: Int,
        code: String,
    ): ApiException = ApiException.from(status, ApiErrorBody(code = code, message = "refused"))
}
