package ru.tinyops.turboist.nativeapp.auth.passkey

import android.content.Context
import androidx.credentials.CreateCredentialResponse
import androidx.credentials.CreatePublicKeyCredentialRequest
import androidx.credentials.CreatePublicKeyCredentialResponse
import androidx.credentials.Credential
import androidx.credentials.CredentialManager
import androidx.credentials.GetCredentialRequest
import androidx.credentials.GetPublicKeyCredentialOption
import androidx.credentials.PublicKeyCredential

/**
 * The device's side of a passkey ceremony.
 *
 * Both halves take the ceremony options as JSON and answer with the
 * authenticator's reply as JSON, because that is the form the server speaks at
 * both ends: it hands out options and verifies a signature computed over the
 * exact bytes it will reconstruct from the reply. Mapping either one through
 * typed fields would risk rewriting something the signature covers.
 *
 * It is an interface so the rules around a ceremony — what a cancellation does
 * to the session, that an assertion never leads to a second-factor step — can be
 * tested without a device to prompt on.
 */
interface PasskeyAuthenticator {
    /**
     * Proves possession of a credential the device already holds.
     *
     * Usernameless: the options ask for a discoverable credential, so the
     * authenticator picks the account and reports it in the answer.
     */
    suspend fun assertion(optionsJson: String): String

    /** Creates a credential and answers with the attestation to be verified. */
    suspend fun registration(optionsJson: String): String
}

/**
 * The platform implementation, on top of Credential Manager.
 *
 * The context has to be the one hosting the visible activity: the system draws
 * the credential sheet over it, and the ceremony belongs to whatever the user is
 * looking at.
 *
 * Nothing here interprets a failure. Credential Manager reports a dismissed
 * sheet, a device with no matching credential and a relying party that does not
 * vouch for this app as three different exceptions, and each of them needs a
 * different sentence on screen, so they are left to travel intact.
 */
class CredentialManagerAuthenticator(
    private val context: Context,
) : PasskeyAuthenticator {
    override suspend fun assertion(optionsJson: String): String {
        val response = CredentialManager.create(context).getCredential(context, assertionRequest(optionsJson))
        return assertionAnswerOf(response.credential)
    }

    override suspend fun registration(optionsJson: String): String {
        val response = CredentialManager.create(context).createCredential(context, registrationRequest(optionsJson))
        return registrationAnswerOf(response)
    }
}

/**
 * The request that asks the device to prove a credential it already holds.
 *
 * The server's options travel into it as the string they arrived as: the
 * signature is computed over bytes the server reconstructs from them, so
 * anything that re-serialized them could invalidate what it signs.
 */
internal fun assertionRequest(optionsJson: String): GetCredentialRequest =
    GetCredentialRequest(listOf(GetPublicKeyCredentialOption(optionsJson)))

/** The request that asks the device to create a credential, options untouched. */
internal fun registrationRequest(optionsJson: String): CreatePublicKeyCredentialRequest =
    CreatePublicKeyCredentialRequest(optionsJson)

/**
 * The assertion JSON out of whatever the platform returned.
 *
 * Credential Manager answers with a credential of whichever kind the chosen
 * provider produced, and a password would be one of them. Posting anything but
 * a public-key answer to the ceremony endpoint could only be refused, so it is
 * refused here, where the reason is still known.
 */
internal fun assertionAnswerOf(credential: Credential): String {
    require(credential is PublicKeyCredential) {
        "the device answered a passkey request with a credential of another kind"
    }
    return credential.authenticationResponseJson
}

/** The attestation JSON out of whatever the platform created, same rule. */
internal fun registrationAnswerOf(response: CreateCredentialResponse): String {
    require(response is CreatePublicKeyCredentialResponse) {
        "the device created a credential of another kind"
    }
    return response.registrationResponseJson
}
