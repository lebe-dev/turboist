package ru.tinyops.turboist.nativeapp.auth.passkey

import androidx.credentials.exceptions.CreateCredentialCancellationException
import androidx.credentials.exceptions.CreateCredentialNoCreateOptionException
import androidx.credentials.exceptions.CreateCredentialProviderConfigurationException
import androidx.credentials.exceptions.CreateCredentialUnsupportedException
import androidx.credentials.exceptions.GetCredentialCancellationException
import androidx.credentials.exceptions.GetCredentialProviderConfigurationException
import androidx.credentials.exceptions.GetCredentialUnsupportedException
import androidx.credentials.exceptions.NoCredentialException
import androidx.credentials.exceptions.domerrors.AbortError
import androidx.credentials.exceptions.domerrors.DataError
import androidx.credentials.exceptions.domerrors.DomError
import androidx.credentials.exceptions.domerrors.NotAllowedError
import androidx.credentials.exceptions.domerrors.SecurityError
import androidx.credentials.exceptions.domerrors.TimeoutError
import androidx.credentials.exceptions.publickeycredential.CreatePublicKeyCredentialDomException
import androidx.credentials.exceptions.publickeycredential.GetPublicKeyCredentialDomException
import ru.tinyops.turboist.core.model.diagnostics.FaultSeverity
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException

/**
 * Why a passkey ceremony did not end in a credential.
 *
 * A passkey has more ways to not work than a password does, and they need
 * different sentences: a dismissed sheet is not a failure, a device that holds
 * no credential for this account is not a broken setup, and a server the device
 * refuses to trust is a configuration problem no amount of retrying fixes.
 * Telling them apart is the difference between a user who knows what to do next
 * and one who taps the same button again.
 *
 * Every one of them leaves the password in place, which is what the wording for
 * each says: a passkey is an additional way in, never the only one.
 */
enum class PasskeyProblem {
    /** The user closed the platform sheet, or let it time out. */
    Cancelled,

    /** This device holds no passkey for this account. */
    NotEnrolled,

    /**
     * The device would not accept the server as the credential's owner — the
     * relying party does not vouch for this app, or vouches for a different
     * signing certificate.
     */
    Untrusted,

    /** This device has no way to handle passkeys at all. */
    Unsupported,

    /** The device produced an answer and the server did not accept it. */
    Refused,

    /** This device already has a passkey for the account. */
    AlreadyRegistered,

    /** The account holds as many passkeys as the server allows. */
    LimitReached,

    /** The server could not be reached, so the ceremony could not be finished. */
    Unreachable,

    /** Anything else. The password path still works, which is what the user is told. */
    Failed,
}

/**
 * Reads a failed ceremony as the sentence the user needs.
 *
 * Two very different sources feed in and both end up here: the platform, which
 * refuses before anything is sent, and the server, which refuses what was sent.
 * A refusal from the server is deliberately *not* read as "wrong credentials" —
 * the user typed nothing, so there is nothing for them to correct, and the
 * useful answer is that the passkey did not work and the password still does.
 */
fun passkeyProblemOf(error: Throwable): PasskeyProblem =
    when {
        error is GetCredentialCancellationException -> PasskeyProblem.Cancelled
        error is CreateCredentialCancellationException -> PasskeyProblem.Cancelled
        error is NoCredentialException -> PasskeyProblem.NotEnrolled
        error is CreateCredentialNoCreateOptionException -> PasskeyProblem.NotEnrolled
        error is GetCredentialUnsupportedException -> PasskeyProblem.Unsupported
        error is CreateCredentialUnsupportedException -> PasskeyProblem.Unsupported
        error is GetCredentialProviderConfigurationException -> PasskeyProblem.Unsupported
        error is CreateCredentialProviderConfigurationException -> PasskeyProblem.Unsupported
        // A relying party the device cannot verify against this app surfaces as a
        // security error from the ceremony: the association the server publishes
        // does not name this package, or names a different signing certificate.
        error is GetPublicKeyCredentialDomException && error.domError is SecurityError -> PasskeyProblem.Untrusted
        error is CreatePublicKeyCredentialDomException && error.domError is SecurityError -> PasskeyProblem.Untrusted
        // The same refusal as the security error above, reported by the platform
        // instead of the web standard: Credential Manager checks that the relying
        // party vouches for this app *before* it draws the sheet, and a check it
        // could not complete arrives as a data error — its own words are "RP ID
        // cannot be validated" — not as a security one. Nothing reached an
        // authenticator, so retrying is pointless and the sentence is the same.
        error is GetPublicKeyCredentialDomException && error.domError is DataError -> PasskeyProblem.Untrusted
        error is CreatePublicKeyCredentialDomException && error.domError is DataError -> PasskeyProblem.Untrusted
        // A sheet the user closed, or let time out, is reported by the platform as
        // the web standard's "not allowed" error rather than as a cancellation.
        passkeyDomErrorOf(error)?.endsTheCeremonyQuietly() == true -> PasskeyProblem.Cancelled
        error is ApiException.Network -> PasskeyProblem.Unreachable
        error is ApiException && error.code == ApiErrorCodes.PASSKEY_EXISTS -> PasskeyProblem.AlreadyRegistered
        error is ApiException && error.code == ApiErrorCodes.LIMIT_EXCEEDED -> PasskeyProblem.LimitReached
        error is ApiException && error.code == ApiErrorCodes.PASSKEY_CEREMONY_INVALID -> PasskeyProblem.Refused
        // A passkey route that is not there at all is an instance with WebAuthn
        // switched off; the offer should not have been shown, and the password is
        // the way in.
        error is ApiException && error.status == NOT_FOUND -> PasskeyProblem.Unsupported
        error is ApiException.Auth -> PasskeyProblem.Refused
        else -> PasskeyProblem.Failed
    }

/** The status a server that never brought WebAuthn up answers these routes with. */
private const val NOT_FOUND = 404

private fun DomError.endsTheCeremonyQuietly(): Boolean =
    this is NotAllowedError || this is AbortError || this is TimeoutError

/**
 * The platform's own name for why a ceremony failed, when it gave one.
 *
 * Only the class name of the standard error is returned — never its message —
 * so it is safe to attach to a report and tells which case was not recognised.
 */
fun passkeyDomErrorName(error: Throwable): String? = passkeyDomErrorOf(error)?.let { it::class.java.simpleName }

private fun passkeyDomErrorOf(error: Throwable): DomError? =
    when (error) {
        is GetPublicKeyCredentialDomException -> error.domError
        is CreatePublicKeyCredentialDomException -> error.domError
        else -> null
    }

/**
 * Whether this is a fault worth a report, as opposed to one of the ways a passkey
 * is expected not to work.
 *
 * A dismissed sheet, a phone holding no credential for the account, a server
 * that refused the assertion and a device without passkey support are all the
 * feature behaving correctly, and reporting them would bury the two cases that
 * are not: a ceremony that failed for a reason nobody chose, and a relying party
 * this app cannot be used with at all — a setup problem on the server, which the
 * user cannot fix and only its owner can.
 */
val PasskeyProblem.isFault: Boolean
    get() = this == PasskeyProblem.Failed || this == PasskeyProblem.Untrusted

/**
 * How much a report about this problem matters. Only asked of an [isFault].
 *
 * A misconfigured instance is handled and stated on screen, so it is a warning;
 * anything else that ended in [PasskeyProblem.Failed] is unexplained by design.
 */
val PasskeyProblem.faultSeverity: FaultSeverity
    get() = if (this == PasskeyProblem.Failed) FaultSeverity.ERROR else FaultSeverity.WARNING
