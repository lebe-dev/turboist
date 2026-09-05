package ru.tinyops.turboist.nativeapp.auth

import android.util.Log
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.ServerUrl
import ru.tinyops.turboist.core.network.dto.LoginOutcome
import ru.tinyops.turboist.core.network.dto.SessionDto
import ru.tinyops.turboist.core.network.dto.optionsJson
import ru.tinyops.turboist.core.network.dto.passkeyCredentialOf
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyAuthenticator
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyOutcome
import ru.tinyops.turboist.nativeapp.auth.passkey.passkeyProblemOf
import ru.tinyops.turboist.nativeapp.di.ApplicationScope
import ru.tinyops.turboist.nativeapp.session.OfflineSessionSource
import ru.tinyops.turboist.nativeapp.session.SessionState
import ru.tinyops.turboist.nativeapp.session.SessionStateSource
import java.util.concurrent.atomic.AtomicBoolean
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Owns the session: which server, which tokens, and therefore which screens the
 * user may see.
 *
 * Everything about being signed in converges here on purpose. The rotating
 * refresh token has exactly one owner, so two paths can never spend it at once
 * — the server treats a token presented twice as stolen and kills the session,
 * which would log the user out for the crime of opening two screens. The HTTP
 * layer's repair hook, the launch sequence and the sign-in screens all rotate
 * through [renew], behind one lock.
 *
 * The rules it enforces, in one place:
 *
 * - a server that cannot be reached is not a server that said no. The stored
 *   token survives it and the app opens on the replica;
 * - a token the server actually rejected is forgotten immediately, but the
 *   replica and the queue of unsent writes are not: signing back in resumes
 *   them rather than starting from an empty device;
 * - a session left unproven that way does not stay unproven: the network coming
 *   back re-checks it, and so does the first request that reaches the server;
 * - every rotation is stored before the new session is used, because the token
 *   just spent is already dead on the server.
 */
@Singleton
class SessionManager
    @Inject
    constructor(
        private val gateway: AuthGateway,
        private val accessTokens: SessionTokens,
        private val refreshTokens: RefreshTokenStore,
        private val serverAddresses: ServerAddressStore,
        private val serverUrl: ServerUrl,
        private val replica: LocalReplica,
        private val networks: NetworkAvailability,
        private val cleartext: CleartextPolicy,
        @param:ApplicationScope private val scope: CoroutineScope,
    ) : SessionStateSource, OfflineSessionSource {
        private val sessionState = MutableStateFlow<SessionState>(SessionState.Connecting)
        private val unverified = MutableStateFlow(false)
        private val step = MutableStateFlow(AuthStep.Connect)
        private val passkeyOffer = MutableStateFlow(false)

        /** Serializes rotation. See the class comment: two rotations is a sign-out. */
        private val rotation = Mutex()

        private val started = AtomicBoolean(false)

        /**
         * The ticket that authorises finishing a two-step sign-in. Short-lived,
         * single-use and bound to this client, so it is held in memory only and
         * dropped the moment the sign-in ends either way.
         */
        private var otpTicket: String? = null

        override val state: StateFlow<SessionState> = sessionState.asStateFlow()

        override val unverifiedSession: StateFlow<Boolean> = unverified.asStateFlow()

        /** Which sign-in screen applies. Only meaningful while the session is signed out. */
        val authStep: StateFlow<AuthStep> = step.asStateFlow()

        /**
         * Whether the sign-in screen should offer a passkey.
         *
         * False until the server has been asked, and false again for a server
         * that never brought WebAuthn up or holds no enrolled credential — an
         * offer that can only end in a shrug is worse than no offer.
         */
        val passkeySignInOffered: StateFlow<Boolean> = passkeyOffer.asStateFlow()

        /**
         * Resolves the stored session, once per process.
         *
         * Called from the application object rather than from a screen: the
         * decision is what the first screen *is*, so it cannot be made by one.
         */
        fun start() {
            if (!started.compareAndSet(false, true)) return
            scope.launch { boot() }
            // A launch with no network leaves a stored session the app is showing
            // but could not prove. Nothing else in the app would ever ask again on
            // its own, so the moment a network appears the session is re-checked.
            scope.launch { networks.whenAvailable().collect { revalidateSession() } }
        }

        /** The launch sequence, exposed for tests that drive it without a process. */
        suspend fun boot() {
            sessionState.value = SessionState.Connecting
            val configured = restoreServerAddress()
            val outcome = if (configured) renewSession() else RenewOutcome.NO_TOKEN
            val setup =
                if (configured && outcome == RenewOutcome.NO_TOKEN) gateway.probeSetup() else SetupProbe.UNKNOWN
            apply(bootDecision(configured, outcome, setup))
        }

        /**
         * Points the app at a server.
         *
         * The address is proved before it is stored: an unreachable or wrong
         * address is rejected here rather than remembered and then failing on
         * every screen afterwards. On failure the previously configured address —
         * if any — is put back, so a mistyped change cannot disconnect a working
         * installation.
         */
        suspend fun connect(raw: String): AuthResult {
            val address =
                when (val read = readServerAddress(raw, allowCleartext = cleartext.allowed)) {
                    is ServerAddress.Accepted -> read.url
                    ServerAddress.Unreadable -> return AuthResult.Failed(AuthFailure.Unreadable)
                    ServerAddress.NotEncrypted -> return AuthResult.Failed(AuthFailure.NotEncrypted)
                }

            val previous = serverUrl.value
            serverUrl.set(address.toString())
            try {
                gateway.ping()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Log.i(TAG, "The address entered on the connect screen did not answer as a Turboist server", e)
                if (previous == null) serverUrl.clear() else serverUrl.set(previous.toString())
                return AuthResult.Failed(AuthFailure.Unreachable)
            }

            serverAddresses.write(address.toString())
            val next = if (gateway.probeSetup() == SetupProbe.REQUIRED) AuthStep.Setup else AuthStep.SignIn
            step.value = next
            return AuthResult.MovedTo(next)
        }

        /** Creates the single account this server hosts and signs in with it. */
        suspend fun createAccount(
            username: String,
            password: String,
        ): AuthResult =
            attempt {
                establish(gateway.createAccount(username, password))
                AuthResult.Done
            }

        suspend fun signIn(
            username: String,
            password: String,
        ): AuthResult =
            attempt {
                when (val outcome = gateway.signIn(username, password)) {
                    is LoginOutcome.Session -> {
                        establish(outcome.session)
                        AuthResult.Done
                    }

                    is LoginOutcome.OtpRequired -> {
                        otpTicket = outcome.ticket
                        AuthResult.OtpRequired
                    }
                }
            }

        /** Finishes a two-step sign-in with a time-based code or a recovery code. */
        suspend fun verifyOtp(code: String): AuthResult {
            val ticket = otpTicket ?: return AuthResult.Failed(AuthFailure.InvalidCode)
            return attempt {
                establish(gateway.verifyOtp(ticket, code))
                AuthResult.Done
            }
        }

        /**
         * Asks the server whether a passkey sign-in is worth offering.
         *
         * Never throws and never shows a failure: this is a question about what
         * to draw, and a server that cannot answer it simply gets the password
         * form, which is the way in that always works.
         */
        suspend fun refreshPasskeyOffer() {
            passkeyOffer.value =
                try {
                    val support = gateway.passkeySupport()
                    support.enabled && support.available
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    Log.i(TAG, "The server did not say whether it offers passkeys; offering the password form only", e)
                    false
                }
        }

        /**
         * Signs in with a passkey.
         *
         * No second-factor step follows, ever. The authenticator holds the key
         * and verified the user, so the assertion is already two factors, and the
         * server answers it with a finished session rather than a challenge —
         * which is why this path establishes the session directly instead of
         * going through the outcome the password login has to branch on.
         *
         * A failure here never disturbs an existing session or the stored token:
         * the password is the recovery path and stays available, so the worst
         * case is a sentence on the sign-in screen.
         */
        suspend fun signInWithPasskey(authenticator: PasskeyAuthenticator): PasskeyOutcome =
            try {
                val ceremony = gateway.beginPasskeyLogin()
                val assertion = authenticator.assertion(ceremony.optionsJson())
                establish(gateway.finishPasskeyLogin(ceremony.ceremonyId, passkeyCredentialOf(assertion)))
                PasskeyOutcome.SignedIn
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Log.i(TAG, "A passkey sign-in did not complete", e)
                PasskeyOutcome.Failed(passkeyProblemOf(e))
            }

        /** Abandons a two-step sign-in and goes back to the username and password. */
        fun cancelOtp() {
            otpTicket = null
        }

        /**
         * Goes back to the address screen.
         *
         * The configured address is left in place until a new one is proved: a
         * user who opens this screen and changes their mind must not come back to
         * an app that has forgotten where its server is.
         */
        fun changeServer() {
            step.value = AuthStep.Connect
        }

        /**
         * Signs out of this device.
         *
         * Everything local goes with the session, so anything the server has not
         * accepted yet would vanish silently. [confirmDiscard] is therefore asked
         * first, with the number at stake, and answering no leaves the session and
         * the queue exactly as they were — a user who reconsiders can still let
         * the queue drain.
         *
         * The server call is best-effort: a device that cannot reach the server
         * must still be able to sign out of it, and the session dies with its
         * refresh token anyway.
         *
         * The server address stays. It is configuration the user chose, not a
         * credential they proved, and forgetting it would make signing back in
         * start by typing a URL.
         *
         * @return false when the user declined at the confirmation.
         */
        suspend fun logOut(confirmDiscard: suspend (Int) -> Boolean): Boolean =
            endSession(confirmDiscard) { gateway.revokeSession() }

        /**
         * Signs every device out, this one included.
         *
         * For the user who suspects a session they cannot identify: rather than
         * picking devices off a list one by one, every way in is closed at once
         * and each device has to sign in again.
         *
         * Locally it is an ordinary sign-out and asks the same question first,
         * because it costs this device the same thing — the replica goes, and
         * with it anything the server has not accepted.
         *
         * @return false when the user declined at the confirmation.
         */
        suspend fun logOutEverywhere(confirmDiscard: suspend (Int) -> Boolean): Boolean =
            endSession(confirmDiscard) { gateway.revokeEverySession() }

        /**
         * The sign-out both flavours share: ask, revoke on the server, then empty
         * the device.
         *
         * The revocation is best-effort. A device that cannot reach the server
         * must still be able to sign out of it, and the session dies with its
         * refresh token anyway — so a failure here is logged and the local half
         * runs regardless, which is the half the user is standing in front of.
         */
        private suspend fun endSession(
            confirmDiscard: suspend (Int) -> Boolean,
            revoke: suspend () -> Unit,
        ): Boolean {
            val unsent = replica.unsentChangeCount()
            if (unsent > 0 && !confirmDiscard(unsent)) return false

            try {
                revoke()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Log.i(TAG, "Signing out could not reach the server; the local session was ended anyway", e)
            }

            accessTokens.set(null)
            refreshTokens.clear()
            otpTicket = null
            replica.wipe()
            unverified.value = false
            step.value = AuthStep.SignIn
            sessionState.value = SessionState.LoggedOut
            return true
        }

        /**
         * Points the app at nothing at all.
         *
         * Stronger than signing out, and deliberately so: the address goes too,
         * so the app comes back at the screen that asks which server to talk to.
         * The replica is a copy of one installation's data, and an id in it means
         * nothing anywhere else, so carrying it across to another server would
         * merge two unrelated workspaces. Emptying it is not tidiness here, it is
         * the only correct outcome.
         *
         * Whatever the server has not accepted goes with it. The caller is
         * expected to have said so and been told yes — this call does not ask.
         *
         * The revocation is best-effort for the same reason signing out's is: a
         * device that cannot reach the server must still be able to walk away
         * from it, and the session dies with its refresh token anyway.
         */
        suspend fun disconnect() {
            try {
                gateway.revokeSession()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Log.i(TAG, "Disconnecting could not reach the server; the local session was ended anyway", e)
            }

            accessTokens.set(null)
            refreshTokens.clear()
            otpTicket = null
            replica.wipe()
            serverAddresses.clear()
            serverUrl.clear()
            unverified.value = false
            step.value = AuthStep.Connect
            sessionState.value = SessionState.LoggedOut
        }

        /**
         * Re-checks a session the app is showing but was never able to prove.
         *
         * The launch that could not reach the server kept the refresh token and
         * opened the app on the replica; this is how that ends. It does nothing
         * for a session that is already proved, and nothing when the server is
         * still out of reach — the app simply stays on the replica and the next
         * network event tries again.
         */
        suspend fun revalidateSession() {
            if (!unverified.value) return
            Log.i(TAG, "A network is available again; re-checking the session the app could not prove")
            renewSession(onlyWhileUnverified = true)
        }

        /**
         * Replaces an aged-out access token, for the HTTP layer.
         *
         * Called on an HTTP thread with the rejected request parked, so it blocks
         * by contract. Returning `null` lets the original rejection stand, which is
         * the honest answer both when the server is unreachable and when it refused
         * the refresh token outright — the two differ in what happens to the
         * session, not in what this request gets.
         *
         * [expired] is `null` when the rejected request carried no token at all,
         * which is what every request looks like after a launch that could not
         * reach the server: there is a stored session, and no access token minted
         * from it yet. That is a session to repair, not one to give up on.
         */
        fun renewAccessToken(expired: String?): String? =
            runBlocking {
                if (renewSession(expired) != RenewOutcome.RENEWED) return@runBlocking null
                accessTokens.accessToken()
            }

        /**
         * Rotates the session behind the one lock.
         *
         * A caller that arrives to find the token already replaced takes the new
         * one instead of asking for another rotation: the refresh token it would
         * spend has already been spent, and presenting it again is what the server
         * reads as theft.
         */
        private suspend fun renewSession(
            expired: String? = null,
            onlyWhileUnverified: Boolean = false,
        ): RenewOutcome =
            rotation.withLock {
                // Both shortcuts say the same thing: someone else already did this
                // rotation while this caller waited for the lock, and asking for
                // another would spend a refresh token that is no longer the current
                // one. That is what the server reads as a stolen credential.
                if (onlyWhileUnverified && !unverified.value) return@withLock RenewOutcome.RENEWED
                val current = accessTokens.accessToken()
                if (expired != null && current != null && current != expired) return@withLock RenewOutcome.RENEWED

                val stored = refreshTokens.read() ?: return@withLock RenewOutcome.NO_TOKEN
                try {
                    val session = gateway.renew(stored)
                    // Stored first: the token just spent is already dead on the
                    // server, so a crash between here and the next launch must find
                    // the replacement rather than the corpse.
                    refreshTokens.write(session.refresh)
                    accessTokens.set(session.access)
                    unverified.value = false
                    sessionState.value = SessionState.LoggedIn
                    RenewOutcome.RENEWED
                } catch (e: ApiException.Auth) {
                    Log.i(TAG, "The stored session was refused by the server; signing out", e)
                    forgetRejectedSession()
                    RenewOutcome.REJECTED
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    // Anything that is not an outright refusal — no network, a
                    // gateway error, a throttled attempt — leaves the session alone.
                    Log.i(TAG, "The stored session could not be checked; continuing without a fresh token", e)
                    RenewOutcome.UNREACHABLE
                }
            }

        private suspend fun forgetRejectedSession() {
            accessTokens.set(null)
            refreshTokens.clear()
            unverified.value = false
            step.value = AuthStep.SignIn
            sessionState.value = SessionState.LoggedOut
        }

        /** Publishes a completed sign-in. The rotation is stored before anything uses the session. */
        private suspend fun establish(session: SessionDto) {
            refreshTokens.write(session.refresh)
            accessTokens.set(session.access)
            otpTicket = null
            unverified.value = false
            sessionState.value = SessionState.LoggedIn
        }

        /**
         * Loads the configured address into the HTTP stack.
         *
         * @return whether the app has a server to talk to at all.
         */
        private suspend fun restoreServerAddress(): Boolean {
            if (serverUrl.isConfigured) return true
            val stored = serverAddresses.read() ?: return false
            return try {
                serverUrl.set(stored)
                true
            } catch (e: IllegalArgumentException) {
                Log.w(TAG, "The stored server address is unusable; asking for it again", e)
                serverAddresses.clear()
                false
            }
        }

        private fun apply(decision: BootDecision) {
            when (decision) {
                BootDecision.Connect -> signedOutOn(AuthStep.Connect)
                BootDecision.Setup -> signedOutOn(AuthStep.Setup)
                BootDecision.SignIn -> signedOutOn(AuthStep.SignIn)
                BootDecision.App -> {
                    unverified.value = false
                    sessionState.value = SessionState.LoggedIn
                }

                BootDecision.OfflineApp -> {
                    // The session is stored but unproven. The app renders from the
                    // replica and says so; the first reachable request settles it.
                    unverified.value = true
                    sessionState.value = SessionState.LoggedIn
                }
            }
        }

        private fun signedOutOn(next: AuthStep) {
            unverified.value = false
            step.value = next
            sessionState.value = SessionState.LoggedOut
        }

        /**
         * Runs one credential-bearing call, turning any refusal into a sentence.
         *
         * A server that has no account yet refuses everything on the versioned API
         * with the same code, so a sign-in against a fresh instance is not an
         * error — it is the answer to "which screen should this be", and the flow
         * moves to setup instead of showing a failure.
         */
        private suspend fun attempt(block: suspend () -> AuthResult): AuthResult =
            try {
                block()
            } catch (_: ApiException.SetupRequired) {
                step.value = AuthStep.Setup
                AuthResult.MovedTo(AuthStep.Setup)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Log.i(TAG, "A sign-in attempt was refused", e)
                AuthResult.Failed(authFailureOf(e))
            }

        private companion object {
            const val TAG = "TurboistSession"
        }
    }
