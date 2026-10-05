package ru.tinyops.turboist.nativeapp.account

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.network.dto.TotpEnrolmentDto
import javax.inject.Inject

/** What the second-factor screen has to say once an action has finished. */
enum class TwoFactorMessage {
    ENABLED,
    DISABLED,
}

/** Which form the screen is showing, if any. */
enum class TwoFactorStep {
    /** Nothing in progress: the screen states whether the second factor is on. */
    IDLE,

    /** A secret has been issued and is waiting for a code that proves it was stored. */
    ENROLLING,

    /** The recovery codes are on screen. They are readable now and never again. */
    RECOVERY,

    /** A code is being asked for so the second factor can be switched off. */
    DISABLING,
}

/**
 * What the second-factor screen draws.
 *
 * @property enrolment the pending secret, held for as long as this screen and
 *   not one moment longer. It is live on the server the instant it is issued, so
 *   writing it anywhere would leave a second copy of the very thing the second
 *   factor exists to keep to one place.
 * @property recoveryCodes the single-use codes, on screen once. The server kept
 *   only hashes, so a user who does not record them here has none.
 * @property available false when this deployment does not offer a second factor
 *   at all, in which case there is nothing to offer and the screen says why.
 * @property unreachable the account's current state could not be read, as
 *   opposed to having been read as "off".
 */
data class TwoFactorUiState(
    val loading: Boolean = true,
    val enabled: Boolean = false,
    val available: Boolean = true,
    val busy: Boolean = false,
    val unreachable: Boolean = false,
    val step: TwoFactorStep = TwoFactorStep.IDLE,
    val enrolment: TotpEnrolmentDto? = null,
    val code: String = "",
    val recoveryCodes: List<String> = emptyList(),
    val problem: AccountProblem? = null,
    val message: TwoFactorMessage? = null,
)

/**
 * Drives turning the time-based second factor on and off.
 *
 * Whether it is on is asked of the server every time rather than remembered: it
 * can be switched off from another device, and a remembered answer would offer
 * the wrong button to a user who is trying to secure an account.
 *
 * Two values pass through here that must never reach storage. The pending secret
 * is live from the moment the server issues it, so a copy of it on the device
 * would defeat the point of a second factor kept in a separate app. The recovery
 * codes are shown exactly once, because the server keeps only their hashes;
 * writing them down is the user's job, and the screen says so before showing
 * them. Both live in this object's memory and go when the screen does.
 */
@HiltViewModel
class TwoFactorViewModel
    @Inject
    constructor(
        private val twoFactor: TwoFactorControl,
    ) : ViewModel() {
        private val mutable = MutableStateFlow(TwoFactorUiState())

        val state: StateFlow<TwoFactorUiState> = mutable.asStateFlow()

        init {
            load()
        }

        fun load() {
            viewModelScope.launch {
                mutable.value = mutable.value.copy(loading = true, unreachable = false)
                mutable.value =
                    try {
                        mutable.value.copy(loading = false, enabled = twoFactor.isEnabled(), problem = null)
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        mutable.value.copy(loading = false, unreachable = true, problem = accountProblemOf(e))
                    }
            }
        }

        /**
         * Asks the server for a secret to enrol.
         *
         * A deployment that was never configured for a second factor has no such
         * endpoint, and its refusal is read as "not offered here" rather than as a
         * failure — there is nothing for the user to retry.
         */
        fun beginEnrolment() {
            run {
                val enrolment = twoFactor.begin()
                mutable.value =
                    mutable.value.copy(
                        step = TwoFactorStep.ENROLLING,
                        enrolment = enrolment,
                        code = "",
                    )
            }
        }

        fun editCode(code: String) {
            mutable.value = mutable.value.copy(code = code, problem = null)
        }

        /**
         * Proves the authenticator holds the pending secret.
         *
         * On success the second factor is on and the recovery codes are on
         * screen; the pending secret is dropped at the same moment, having done
         * its job.
         */
        fun confirmEnrolment() {
            val code = mutable.value.code.trim()
            if (code.isEmpty()) return
            run {
                val codes = twoFactor.confirm(code)
                mutable.value =
                    mutable.value.copy(
                        enabled = true,
                        step = TwoFactorStep.RECOVERY,
                        enrolment = null,
                        code = "",
                        recoveryCodes = codes,
                        message = TwoFactorMessage.ENABLED,
                    )
            }
        }

        /**
         * Walks away from an enrolment.
         *
         * Nothing has to be undone on the server: an unconfirmed secret leaves the
         * account signing in with a password alone, exactly as before.
         */
        fun cancelEnrolment() {
            mutable.value =
                mutable.value.copy(step = TwoFactorStep.IDLE, enrolment = null, code = "", problem = null)
        }

        /** Closes the recovery codes. They are unreadable from this moment on. */
        fun finishRecovery() {
            mutable.value = mutable.value.copy(step = TwoFactorStep.IDLE, recoveryCodes = emptyList())
        }

        fun startDisabling() {
            mutable.value = mutable.value.copy(step = TwoFactorStep.DISABLING, code = "", problem = null)
        }

        fun cancelDisabling() {
            mutable.value = mutable.value.copy(step = TwoFactorStep.IDLE, code = "", problem = null)
        }

        /** Switches the second factor off. The code may be a current one or a recovery code. */
        fun disable() {
            val code = mutable.value.code.trim()
            if (code.isEmpty()) return
            run {
                twoFactor.disable(code)
                mutable.value =
                    mutable.value.copy(
                        enabled = false,
                        step = TwoFactorStep.IDLE,
                        code = "",
                        message = TwoFactorMessage.DISABLED,
                    )
            }
        }

        fun dismissMessage() {
            mutable.value = mutable.value.copy(problem = null, message = null)
        }

        /**
         * Runs one call, and refuses to start a second while the first is out.
         *
         * A refusal that says the routes are not there settles the whole screen
         * rather than the one attempt: there is no version of this deployment
         * where the next tap works, so the screen stops offering.
         */
        private fun run(block: suspend () -> Unit) {
            if (mutable.value.busy) return
            mutable.value = mutable.value.copy(busy = true, problem = null, message = null)
            viewModelScope.launch {
                try {
                    block()
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    mutable.value =
                        if (endpointIsAbsent(e)) {
                            mutable.value.copy(
                                available = false,
                                step = TwoFactorStep.IDLE,
                                enrolment = null,
                                code = "",
                                problem = AccountProblem.UNAVAILABLE,
                            )
                        } else {
                            mutable.value.copy(problem = accountProblemOf(e))
                        }
                } finally {
                    mutable.value = mutable.value.copy(busy = false)
                }
            }
        }
    }
