package ru.tinyops.turboist.nativeapp.auth.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import ru.tinyops.turboist.nativeapp.auth.AuthFailure
import ru.tinyops.turboist.nativeapp.auth.AuthResult
import ru.tinyops.turboist.nativeapp.auth.AuthStep
import ru.tinyops.turboist.nativeapp.auth.SessionManager
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyAuthenticator
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyOutcome
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyProblem
import javax.inject.Inject

/**
 * What the three sign-in screens draw.
 *
 * @property busy a call is in flight; the form is locked and the button shows it.
 * @property failure why the last attempt did not go through, or `null` after a
 *   fresh edit — an error left standing under a field the user has since changed
 *   describes a state that no longer exists.
 * @property awaitingOtp the password was right and the account wants its second
 *   factor.
 * @property usingRecoveryCode the user chose to type a recovery code instead of
 *   a code from their authenticator. Both go to the same endpoint; only the
 *   wording and the keyboard differ.
 * @property awaitingDevice a passkey ceremony is running and the platform sheet
 *   is up. Distinct from [busy] because the waiting is on the device rather than
 *   on the server, and the button says so.
 * @property passkeyProblem why the last passkey attempt did not end in a
 *   session. Kept apart from [failure]: a passkey has ways to not work that a
 *   password does not, and each of them needs its own sentence.
 */
data class AuthUiState(
    val busy: Boolean = false,
    val failure: AuthFailure? = null,
    val awaitingOtp: Boolean = false,
    val usingRecoveryCode: Boolean = false,
    val awaitingDevice: Boolean = false,
    val passkeyProblem: PasskeyProblem? = null,
)

/**
 * Drives the sign-in screens.
 *
 * It holds no session state of its own: which screen applies and whether there
 * is a session are the session layer's answers, and duplicating them here is how
 * a screen ends up disagreeing with the app about whether the user is signed in.
 * What lives here is only what a *form* has: whether it is waiting, and what to
 * tell the user about the last attempt.
 */
@HiltViewModel
class AuthViewModel
    @Inject
    constructor(
        private val session: SessionManager,
    ) : ViewModel() {
        private val mutable = MutableStateFlow(AuthUiState())

        val state: StateFlow<AuthUiState> = mutable.asStateFlow()

        /** Which screen the flow is on, straight from the session layer. */
        val step: StateFlow<AuthStep> = session.authStep

        /**
         * Whether to offer a passkey at all, straight from the session layer.
         *
         * A server with WebAuthn switched off, or with nothing enrolled yet,
         * offers nothing: a button that can only end in an explanation is worse
         * than no button.
         */
        val passkeyOffered: StateFlow<Boolean> = session.passkeySignInOffered

        init {
            // The answer belongs to the server, so it is asked again whenever the
            // flow arrives at the sign-in screen — which is also how a user who
            // switched servers stops being offered the previous one's passkeys.
            viewModelScope.launch {
                session.authStep.collect { step ->
                    if (step == AuthStep.SignIn) session.refreshPasskeyOffer()
                }
            }
        }

        fun connect(address: String) = attempt { session.connect(address) }

        fun createAccount(
            username: String,
            password: String,
            confirmation: String,
        ) {
            // Checked here rather than at the server: it is the one refusal that
            // needs no round trip, and typing a password twice only to wait for a
            // network call to say they differ is an unkind way to learn it.
            if (password != confirmation) {
                mutable.value = mutable.value.copy(failure = AuthFailure.PasswordsDoNotMatch)
                return
            }
            attempt { session.createAccount(username, password) }
        }

        fun signIn(
            username: String,
            password: String,
        ) = attempt { session.signIn(username, password) }

        fun verifyOtp(code: String) = attempt { session.verifyOtp(code) }

        /**
         * Signs in with a passkey, with the platform sheet drawn over the screen
         * the [authenticator] was built from.
         *
         * There is deliberately no second-factor branch: an assertion proves both
         * possession and user verification at once, so the only two ways this
         * ends are a session and a sentence.
         */
        fun signInWithPasskey(authenticator: PasskeyAuthenticator) {
            if (mutable.value.busy) return
            mutable.value =
                mutable.value.copy(busy = true, awaitingDevice = true, failure = null, passkeyProblem = null)
            viewModelScope.launch {
                mutable.value =
                    when (val outcome = session.signInWithPasskey(authenticator)) {
                        PasskeyOutcome.SignedIn -> AuthUiState()
                        is PasskeyOutcome.Failed ->
                            mutable.value.copy(
                                busy = false,
                                awaitingDevice = false,
                                passkeyProblem = outcome.problem,
                            )
                    }
            }
        }

        /** Goes back from the second-factor step to the username and password. */
        fun cancelOtp() {
            session.cancelOtp()
            mutable.value = AuthUiState()
        }

        fun useRecoveryCode(using: Boolean) {
            mutable.value = mutable.value.copy(usingRecoveryCode = using, failure = null)
        }

        fun changeServer() {
            session.changeServer()
            mutable.value = AuthUiState()
        }

        /** Clears a message about an attempt the user has since edited away from. */
        fun clearFailure() {
            val current = mutable.value
            if (current.failure == null && current.passkeyProblem == null) return
            mutable.value = current.copy(failure = null, passkeyProblem = null)
        }

        /**
         * Runs one attempt, and refuses to start a second while the first is out.
         *
         * Double submission matters more here than on an ordinary form: two
         * sign-ins in flight take two sessions out of this client's budget, and on
         * a slow link a user watching nothing happen will press the button again.
         */
        private fun attempt(block: suspend () -> AuthResult) {
            if (mutable.value.busy) return
            mutable.value = mutable.value.copy(busy = true, failure = null)
            viewModelScope.launch {
                mutable.value =
                    when (val result = block()) {
                        is AuthResult.Failed -> mutable.value.copy(busy = false, failure = result.failure)
                        AuthResult.OtpRequired ->
                            mutable.value.copy(busy = false, failure = null, awaitingOtp = true)
                        // A session, or a move to another screen: either way the
                        // form is done and must not keep a stale second-factor step.
                        else -> AuthUiState()
                    }
            }
        }
    }
