package ru.tinyops.turboist.nativeapp.account

import ru.tinyops.turboist.core.network.api.ApiTokenApi
import ru.tinyops.turboist.core.network.api.AuthApi
import ru.tinyops.turboist.core.network.api.SessionApi
import ru.tinyops.turboist.core.network.api.TotpApi
import ru.tinyops.turboist.core.network.dto.ActiveSessionDto
import ru.tinyops.turboist.core.network.dto.ApiTokenDto
import ru.tinyops.turboist.core.network.dto.CreateApiTokenRequest
import ru.tinyops.turboist.core.network.dto.CreatedApiTokenDto
import ru.tinyops.turboist.core.network.dto.TotpCodeRequest
import ru.tinyops.turboist.core.network.dto.TotpEnrolmentDto
import ru.tinyops.turboist.nativeapp.auth.LocalReplica
import ru.tinyops.turboist.nativeapp.auth.SessionManager
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The three administrative surfaces of the account, as narrow ports.
 *
 * All of them are live-only, and that is a decision rather than an omission.
 * Which devices can reach the account, which tokens exist and whether a second
 * factor is on are questions whose answers are true only at the instant the
 * server gives them; a copy kept on the phone would go on insisting a revoked
 * session is open, and a screen that showed it would be worse than one that
 * admits it cannot say. So nothing on these paths is written to the replica, to
 * a preference file, or anywhere else on disk — the answers live for as long as
 * the screen showing them, and go with it.
 *
 * They are ports rather than the endpoint interfaces because two of the three
 * need something the HTTP layer does not have: signing every device out ends
 * this device's own session, and that is the session layer's business.
 */
interface SessionControl {
    /** The sessions that can reach the account right now, with this one flagged. */
    suspend fun list(): List<ActiveSessionDto>

    /** Ends one other session. The device holding it signs in again or does without. */
    suspend fun revoke(id: Long)

    /** Ends every session except this one — for kicking a device left behind. */
    suspend fun signOutOthers()

    /**
     * How many changes this device has made that the server has not accepted.
     *
     * Signing out everywhere signs this device out too, and the wipe takes them
     * with it. The number is part of the question the user is asked, because
     * "some changes will be lost" is not something a person can weigh.
     */
    suspend fun unsentChangeCount(): Int

    /**
     * Ends every session of the account, this device's included.
     *
     * The caller has already asked, so this does not: it signs out and empties
     * the device the way an ordinary sign-out does.
     */
    suspend fun signOutEverywhere()
}

/** The long-lived tokens external tools authenticate with. */
interface ApiTokenControl {
    /** The tokens that exist, as metadata. Their secrets are not readable, ever. */
    suspend fun list(): List<ApiTokenDto>

    /**
     * Mints a token and returns it with its plaintext — the only moment that
     * value exists outside the user's own notes.
     */
    suspend fun create(
        name: String,
        scopes: List<String>,
    ): CreatedApiTokenDto

    /** Revokes a token; whatever authenticates with it stops working at once. */
    suspend fun delete(id: Long)
}

/** The time-based second factor: whether it is on, and turning it on or off. */
interface TwoFactorControl {
    /** Whether the account currently asks for a code after the password. */
    suspend fun isEnabled(): Boolean

    /**
     * Starts an enrolment and hands back the secret to show.
     *
     * Nothing is switched on by it: an enrolment the user walks away from leaves
     * the account signing in with a password alone.
     */
    suspend fun begin(): TotpEnrolmentDto

    /**
     * Proves the authenticator holds the pending secret and switches the second
     * factor on, returning the recovery codes. They are shown once and never
     * stored — the server keeps only hashes of them.
     */
    suspend fun confirm(code: String): List<String>

    /** Switches it off. The code may be a current one or an unused recovery code. */
    suspend fun disable(code: String)
}

/** [SessionControl] over the API, plus the local half of signing this device out. */
@Singleton
class HttpSessionControl
    @Inject
    constructor(
        private val api: SessionApi,
        private val auth: AuthApi,
        private val replica: LocalReplica,
        private val session: SessionManager,
    ) : SessionControl {
        override suspend fun list(): List<ActiveSessionDto> = api.list()

        override suspend fun revoke(id: Long) {
            api.revoke(id)
        }

        override suspend fun signOutOthers() {
            auth.logoutOthers()
        }

        override suspend fun unsentChangeCount(): Int = replica.unsentChangeCount()

        override suspend fun signOutEverywhere() {
            // The confirmation has already been given by the screen, with the
            // number of unsent changes in it, so nothing is asked again here.
            session.logOutEverywhere { true }
        }
    }

/** [ApiTokenControl] over the API. Nothing it returns is written down. */
@Singleton
class HttpApiTokenControl
    @Inject
    constructor(
        private val api: ApiTokenApi,
    ) : ApiTokenControl {
        override suspend fun list(): List<ApiTokenDto> = api.list()

        override suspend fun create(
            name: String,
            scopes: List<String>,
        ): CreatedApiTokenDto = api.create(CreateApiTokenRequest(name = name.trim(), scopes = scopes))

        override suspend fun delete(id: Long) {
            api.delete(id)
        }
    }

/**
 * [TwoFactorControl] over the API.
 *
 * Whether the second factor is on is asked of the server rather than remembered:
 * it can be turned off from another device, and a remembered answer would offer
 * the user the wrong button.
 */
@Singleton
class HttpTwoFactorControl
    @Inject
    constructor(
        private val api: TotpApi,
        private val auth: AuthApi,
    ) : TwoFactorControl {
        override suspend fun isEnabled(): Boolean = auth.me().user.totpEnabled

        override suspend fun begin(): TotpEnrolmentDto = api.setup()

        override suspend fun confirm(code: String): List<String> =
            api.confirm(TotpCodeRequest(code.trim())).recoveryCodes

        override suspend fun disable(code: String) {
            api.disable(TotpCodeRequest(code.trim()))
        }
    }
