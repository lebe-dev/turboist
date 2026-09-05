package ru.tinyops.turboist.nativeapp.auth

import kotlinx.serialization.json.JsonObject
import ru.tinyops.turboist.core.model.ClientKind
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.api.AuthApi
import ru.tinyops.turboist.core.network.api.PasskeyApi
import ru.tinyops.turboist.core.network.dto.LoginOutcome
import ru.tinyops.turboist.core.network.dto.LoginRequest
import ru.tinyops.turboist.core.network.dto.OtpLoginRequest
import ru.tinyops.turboist.core.network.dto.PasskeyCeremonyDto
import ru.tinyops.turboist.core.network.dto.PasskeyConfigDto
import ru.tinyops.turboist.core.network.dto.PasskeyLoginBeginRequest
import ru.tinyops.turboist.core.network.dto.PasskeyLoginFinishRequest
import ru.tinyops.turboist.core.network.dto.RefreshRequest
import ru.tinyops.turboist.core.network.dto.SessionDto
import ru.tinyops.turboist.core.network.dto.SetupRequest
import ru.tinyops.turboist.core.network.dto.toOutcome
import javax.inject.Inject
import javax.inject.Singleton

/** Whether the instance still has to have its one account created. */
enum class SetupProbe {
    /** No account exists yet, so the only thing that can be done is create one. */
    REQUIRED,

    /** An account exists; the way in is signing in. */
    DONE,

    /** The server did not answer, so neither screen can be ruled out. */
    UNKNOWN,
}

/**
 * The sign-in endpoints as the session layer needs them.
 *
 * A narrow port rather than the generated endpoint interface, for two reasons.
 * It is the one place that knows this client identifies itself as an Android
 * client, so no screen can forget to say so and quietly take a session out of
 * the wrong per-client budget. And it lets the session rules — rotation,
 * boot decisions, the logout wipe — be tested against a hand-written stand-in
 * instead of against HTTP.
 */
interface AuthGateway {
    /**
     * Confirms an address really is a Turboist server.
     *
     * Deliberately the unauthenticated endpoint: it runs before any credentials
     * exist, so a typo cannot send a password to whatever answered.
     */
    suspend fun ping()

    /** Asks whether the account still has to be created. Never throws. */
    suspend fun probeSetup(): SetupProbe

    /** Creates the single account this installation has. */
    suspend fun createAccount(
        username: String,
        password: String,
    ): SessionDto

    /** Signs in, which either produces a session or a demand for the second factor. */
    suspend fun signIn(
        username: String,
        password: String,
    ): LoginOutcome

    /** Finishes a two-step sign-in with a time-based code or a recovery code. */
    suspend fun verifyOtp(
        ticket: String,
        code: String,
    ): SessionDto

    /**
     * Whether this server offers a passkey sign-in at all: whether it brought
     * WebAuthn up, and whether anyone has enrolled a credential yet. Asked from
     * the unauthenticated probe, because the answer decides what the sign-in
     * screen shows before anyone has signed in.
     */
    suspend fun passkeySupport(): PasskeyConfigDto

    /**
     * Starts a usernameless passkey login. The credential is resident on the
     * device, so nothing here names the account.
     */
    suspend fun beginPasskeyLogin(): PasskeyCeremonyDto

    /**
     * Finishes a passkey login, and answers with a complete session.
     *
     * Never with a second-factor challenge: the authenticator holds the key and
     * verified the user, so the assertion already carries both factors.
     */
    suspend fun finishPasskeyLogin(
        ceremonyId: String,
        credential: JsonObject,
    ): SessionDto

    /**
     * Rotates the session. The token presented here dies with the call, so the
     * replacement has to be stored before anything else uses the session.
     */
    suspend fun renew(refreshToken: String): SessionDto

    /** Ends this session on the server. Other devices stay signed in. */
    suspend fun revokeSession()

    /**
     * Ends every session of this account, including the one making the call.
     *
     * The way out for a user who thinks somebody else is holding a session they
     * cannot identify: rather than picking devices off a list, everything is
     * revoked at once and each device signs in again.
     */
    suspend fun revokeEverySession()
}

/** The [AuthGateway] backed by the app's one HTTP stack. */
@Singleton
class HttpAuthGateway
    @Inject
    constructor(
        private val api: AuthApi,
        private val passkeys: PasskeyApi,
    ) : AuthGateway {
        override suspend fun ping() {
            api.publicConfig()
        }

        override suspend fun probeSetup(): SetupProbe {
            // The question is answered by the refusal, not by the body: the setup
            // gate sits in front of the versioned API and short-circuits it until
            // an account exists. A 401 therefore means "there is an account, you
            // are simply not signed in", which is the answer we want.
            return try {
                api.setupProbe()
                SetupProbe.DONE
            } catch (_: ApiException.SetupRequired) {
                SetupProbe.REQUIRED
            } catch (_: ApiException.Network) {
                SetupProbe.UNKNOWN
            } catch (_: ApiException) {
                SetupProbe.DONE
            }
        }

        override suspend fun createAccount(
            username: String,
            password: String,
        ): SessionDto = api.setup(SetupRequest(username = username, password = password, clientKind = CLIENT_KIND))

        override suspend fun signIn(
            username: String,
            password: String,
        ): LoginOutcome =
            api.login(LoginRequest(username = username, password = password, clientKind = CLIENT_KIND)).toOutcome()

        override suspend fun verifyOtp(
            ticket: String,
            code: String,
        ): SessionDto = api.loginWithOtp(OtpLoginRequest(ticket = ticket, code = code))

        override suspend fun passkeySupport(): PasskeyConfigDto = api.publicConfig().passkeys

        override suspend fun beginPasskeyLogin(): PasskeyCeremonyDto =
            passkeys.loginBegin(PasskeyLoginBeginRequest(clientKind = CLIENT_KIND))

        override suspend fun finishPasskeyLogin(
            ceremonyId: String,
            credential: JsonObject,
        ): SessionDto =
            passkeys.loginFinish(
                PasskeyLoginFinishRequest(
                    ceremonyId = ceremonyId,
                    credential = credential,
                    clientKind = CLIENT_KIND,
                ),
            )

        override suspend fun renew(refreshToken: String): SessionDto = api.refresh(RefreshRequest(refreshToken))

        override suspend fun revokeSession() {
            api.logout()
        }

        override suspend fun revokeEverySession() {
            api.logoutAll()
        }

        private companion object {
            /**
             * How this client names itself. Sessions are capped per client kind, so
             * saying it once here keeps the native app inside its own budget instead
             * of competing with a browser for the same five slots.
             */
            val CLIENT_KIND: String = ClientKind.ANDROID.wire
        }
    }
