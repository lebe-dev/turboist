package ru.tinyops.turboist.nativeapp.account

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.network.dto.ActiveSessionDto
import javax.inject.Inject

/** The three ways of closing a session, each asked about before it happens. */
sealed interface SessionsConfirmation {
    /** Ends one named session. */
    data class Revoke(val session: ActiveSessionDto) : SessionsConfirmation

    /** Ends every session but this one. Nothing on this device changes. */
    data object SignOutOthers : SessionsConfirmation

    /**
     * Ends every session, this one included — so it signs this device out and
     * empties it, which is why the count of unsent changes is part of the
     * question.
     */
    data object SignOutEverywhere : SessionsConfirmation
}

/** What the sessions screen has to say after an action. */
enum class SessionsMessage {
    OTHERS_SIGNED_OUT,
}

/**
 * What the sessions screen draws.
 *
 * @property unreachable the list itself could not be fetched. The screen then
 *   says it needs a connection rather than claiming the account has no sessions
 *   open — the two look alike on screen and mean opposite things.
 * @property unsentChangeCount how much of this device's work the server has not
 *   taken. Only meaningful while the sign-out-everywhere question is up, which
 *   is the one action here that empties this device.
 */
data class SessionsUiState(
    val loading: Boolean = true,
    val sessions: List<ActiveSessionDto> = emptyList(),
    val busy: Boolean = false,
    val unreachable: Boolean = false,
    val problem: AccountProblem? = null,
    val message: SessionsMessage? = null,
    val confirming: SessionsConfirmation? = null,
    val unsentChangeCount: Int = 0,
)

/**
 * Drives the list of ways into the account.
 *
 * Everything here is read live and nothing is kept. A session list is the answer
 * to "who can reach my account right now", and that is only true at the moment
 * the server says it: a copy on the device would go on showing a session the
 * user revoked from another phone, which is precisely the reassurance this
 * screen must never give falsely. So a list that could not be fetched is
 * reported as unfetched, and an empty one is only ever the server's own answer.
 *
 * Every action asks first. Revoking is not undoable and signing out everywhere
 * takes this device with it, so each is a question with its consequence spelled
 * out rather than a button that acts on the first tap.
 */
@HiltViewModel
class SessionsViewModel
    @Inject
    constructor(
        private val sessions: SessionControl,
    ) : ViewModel() {
        private val mutable = MutableStateFlow(SessionsUiState())

        val state: StateFlow<SessionsUiState> = mutable.asStateFlow()

        init {
            load()
        }

        fun load() {
            viewModelScope.launch {
                mutable.value = mutable.value.copy(loading = true, unreachable = false)
                mutable.value =
                    try {
                        mutable.value.copy(loading = false, sessions = sessions.list(), problem = null)
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        mutable.value.copy(loading = false, unreachable = true, problem = accountProblemOf(e))
                    }
            }
        }

        /**
         * Puts a question up.
         *
         * The one that ends this device's own session carries the number of
         * changes the server has not taken, read now rather than when the screen
         * opened: the queue drains in the background, and a number from a minute
         * ago could frighten the user about work that has since gone out.
         */
        fun ask(confirmation: SessionsConfirmation) {
            if (mutable.value.busy) return
            mutable.value = mutable.value.copy(confirming = confirmation, message = null, problem = null)
            if (confirmation != SessionsConfirmation.SignOutEverywhere) return
            viewModelScope.launch {
                val unsent =
                    try {
                        sessions.unsentChangeCount()
                    } catch (e: CancellationException) {
                        throw e
                    } catch (_: Exception) {
                        0
                    }
                mutable.value = mutable.value.copy(unsentChangeCount = unsent)
            }
        }

        fun cancelConfirmation() {
            mutable.value = mutable.value.copy(confirming = null)
        }

        /** Carries out whatever question is up, and closes it either way. */
        fun confirm() {
            val pending = mutable.value.confirming ?: return
            mutable.value = mutable.value.copy(confirming = null)
            when (pending) {
                is SessionsConfirmation.Revoke -> revoke(pending.session)
                SessionsConfirmation.SignOutOthers -> signOutOthers()
                SessionsConfirmation.SignOutEverywhere -> signOutEverywhere()
            }
        }

        fun dismissMessage() {
            mutable.value = mutable.value.copy(problem = null, message = null)
        }

        private fun revoke(session: ActiveSessionDto) {
            run {
                sessions.revoke(session.id)
                // Dropped from the list only once the server has agreed. A row
                // removed on the strength of a call that failed would tell the
                // user a device is locked out when it is still signed in.
                mutable.value = mutable.value.copy(sessions = mutable.value.sessions.filterNot { it.id == session.id })
            }
        }

        private fun signOutOthers() {
            run {
                sessions.signOutOthers()
                mutable.value =
                    mutable.value.copy(
                        sessions = mutable.value.sessions.filter { it.isCurrent },
                        message = SessionsMessage.OTHERS_SIGNED_OUT,
                    )
            }
        }

        /**
         * Signs every device out, this one included.
         *
         * Nothing is reported afterwards on purpose: the session ends, so the app
         * is already on its way back to the sign-in screen and there is no screen
         * left for a message to land on.
         */
        private fun signOutEverywhere() {
            run { sessions.signOutEverywhere() }
        }

        /**
         * Runs one call, and refuses to start a second while the first is out.
         *
         * Two revocations at once is not merely a double submission here: the
         * list each one answers against has moved, and the second would be
         * applied to rows the first already removed.
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
                    mutable.value = mutable.value.copy(problem = accountProblemOf(e))
                } finally {
                    mutable.value = mutable.value.copy(busy = false)
                }
            }
        }
    }
