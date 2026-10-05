package ru.tinyops.turboist.nativeapp.auth.passkey.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.network.dto.PasskeyDto
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyAuthenticator
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyProblem
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyRegistry
import ru.tinyops.turboist.nativeapp.auth.passkey.passkeyProblemOf
import javax.inject.Inject

/**
 * What the passkey screen draws.
 *
 * @property loading the list has not arrived yet. Distinct from an empty list,
 *   which is a real answer and gets its own wording.
 * @property busy a ceremony or a write is out; the screen's actions are locked
 *   so a second tap cannot start a second ceremony.
 * @property awaitingDevice the platform sheet is up, which the add button says
 *   rather than spinning silently behind it.
 * @property problem why the last attempt did not work, or `null`.
 * @property unreachable the list itself could not be fetched. The screen then
 *   says it needs a connection instead of claiming the account has no passkeys.
 * @property added set once after a successful enrolment, so the screen can
 *   confirm it and then forget it.
 */
data class PasskeysUiState(
    val loading: Boolean = true,
    val passkeys: List<PasskeyDto> = emptyList(),
    val busy: Boolean = false,
    val awaitingDevice: Boolean = false,
    val problem: PasskeyProblem? = null,
    val unreachable: Boolean = false,
    val added: Boolean = false,
)

/**
 * Drives the passkey screen.
 *
 * Everything here is online: a passkey cannot be enrolled or revoked without the
 * server agreeing, so there is nothing to queue and nothing to show from a
 * replica. A list that could not be fetched is reported as exactly that, because
 * an empty list would be a lie about which devices can sign in to the account.
 */
@HiltViewModel
class PasskeysViewModel
    @Inject
    constructor(
        private val registry: PasskeyRegistry,
    ) : ViewModel() {
        private val mutable = MutableStateFlow(PasskeysUiState())

        val state: StateFlow<PasskeysUiState> = mutable.asStateFlow()

        init {
            load()
        }

        fun load() {
            viewModelScope.launch {
                mutable.value = mutable.value.copy(loading = true, unreachable = false)
                mutable.value =
                    try {
                        mutable.value.copy(loading = false, passkeys = registry.list())
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        mutable.value.copy(loading = false, unreachable = true, problem = passkeyProblemOf(e))
                    }
            }
        }

        /**
         * Enrols this device.
         *
         * @param name the user's label for it, or blank to let the server name it.
         */
        fun add(
            authenticator: PasskeyAuthenticator,
            name: String,
        ) {
            run(awaitingDevice = true) {
                val stored = registry.register(authenticator, name)
                mutable.value = mutable.value.copy(passkeys = mutable.value.passkeys + stored, added = true)
            }
        }

        fun rename(
            id: Long,
            name: String,
        ) {
            if (name.isBlank()) return
            run {
                val renamed = registry.rename(id, name)
                mutable.value =
                    mutable.value.copy(passkeys = mutable.value.passkeys.map { if (it.id == id) renamed else it })
            }
        }

        /**
         * Removes a credential. The last one may go too: the password is the
         * recovery path, so this can never lock the account out.
         */
        fun remove(id: Long) {
            run {
                registry.remove(id)
                mutable.value = mutable.value.copy(passkeys = mutable.value.passkeys.filterNot { it.id == id })
            }
        }

        /** Drops the last message, once the user has seen it. */
        fun dismissMessage() {
            mutable.value = mutable.value.copy(problem = null, added = false)
        }

        /**
         * Runs one call, and refuses to start a second while the first is out.
         *
         * Two ceremonies at once is worse than an ordinary double submission: the
         * platform sheet is modal, and the second call would be answered by
         * whatever the user did to the first sheet.
         */
        private fun run(
            awaitingDevice: Boolean = false,
            block: suspend () -> Unit,
        ) {
            if (mutable.value.busy) return
            mutable.value =
                mutable.value.copy(busy = true, awaitingDevice = awaitingDevice, problem = null, added = false)
            viewModelScope.launch {
                try {
                    block()
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    mutable.value = mutable.value.copy(problem = passkeyProblemOf(e))
                } finally {
                    mutable.value = mutable.value.copy(busy = false, awaitingDevice = false)
                }
            }
        }
    }
