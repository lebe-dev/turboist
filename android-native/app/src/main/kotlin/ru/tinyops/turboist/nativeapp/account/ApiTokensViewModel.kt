package ru.tinyops.turboist.nativeapp.account

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.network.dto.ApiTokenDto
import ru.tinyops.turboist.core.network.dto.CreatedApiTokenDto
import javax.inject.Inject

/** The three ready-made permission sets the form offers before the checkboxes. */
enum class ScopePreset {
    FULL_ACCESS,
    READ_ONLY,
    TASKS_FULL,
}

/**
 * What the API tokens screen draws.
 *
 * @property created the token that was just minted, plaintext and all. It exists
 *   here and nowhere else: the server kept only a hash, so once this is dropped
 *   the value is gone for good — which is what the screen warns before showing
 *   it, and why nothing writes it down.
 * @property scopesMissing the user asked for a token with no permissions at all.
 *   The server refuses that, so it is said here while the form is still open.
 * @property unreachable the list could not be fetched, as opposed to being empty.
 */
data class ApiTokensUiState(
    val loading: Boolean = true,
    val tokens: List<ApiTokenDto> = emptyList(),
    val busy: Boolean = false,
    val unreachable: Boolean = false,
    val problem: AccountProblem? = null,
    val name: String = "",
    val selection: ScopeSelection = ScopeSelection.NONE,
    val scopesMissing: Boolean = false,
    val created: CreatedApiTokenDto? = null,
    val deleting: ApiTokenDto? = null,
) {
    /** True when the form has enough to mint a token the server would accept. */
    val canCreate: Boolean get() = !busy && name.isNotBlank()
}

/**
 * Drives the long-lived tokens an external tool authenticates with.
 *
 * The whole screen is live: tokens are listed from the server every time and
 * nothing about them is stored on the device. That is not only a secrecy rule
 * about the plaintext — a stale list of what can reach the account would be the
 * same lie the session list must not tell.
 *
 * The plaintext itself is the strictest case. It exists in exactly one response
 * and is kept in memory for as long as the dialog showing it, then dropped. It
 * is never written to the replica, to a preference file, or to a log, because
 * the server cannot reissue it and anything that stored it would be a copy of a
 * credential nobody could revoke without noticing it had leaked.
 */
@HiltViewModel
class ApiTokensViewModel
    @Inject
    constructor(
        private val tokens: ApiTokenControl,
    ) : ViewModel() {
        private val mutable = MutableStateFlow(ApiTokensUiState())

        val state: StateFlow<ApiTokensUiState> = mutable.asStateFlow()

        init {
            load()
        }

        fun load() {
            viewModelScope.launch {
                mutable.value = mutable.value.copy(loading = true, unreachable = false)
                mutable.value =
                    try {
                        mutable.value.copy(loading = false, tokens = tokens.list(), problem = null)
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        mutable.value.copy(loading = false, unreachable = true, problem = accountProblemOf(e))
                    }
            }
        }

        fun editName(name: String) {
            mutable.value = mutable.value.copy(name = name)
        }

        fun applyPreset(preset: ScopePreset) {
            val selection =
                when (preset) {
                    ScopePreset.FULL_ACCESS -> ScopeSelection.FULL_ACCESS
                    ScopePreset.READ_ONLY -> ScopeSelection.READ_ONLY
                    ScopePreset.TASKS_FULL -> ScopeSelection.TASKS_FULL
                }
            select(selection)
        }

        fun setFullAccess(granted: Boolean) {
            select(mutable.value.selection.withFullAccess(granted))
        }

        fun setRead(
            resource: ScopeResource,
            granted: Boolean,
        ) {
            select(mutable.value.selection.withRead(resource, granted))
        }

        fun setWrite(
            resource: ScopeResource,
            granted: Boolean,
        ) {
            select(mutable.value.selection.withWrite(resource, granted))
        }

        /**
         * Mints a token.
         *
         * The two refusals the server would make are made here instead, so the
         * user hears them while the form is still in front of them: a token needs
         * a name, and it needs at least one permission.
         */
        fun create() {
            val current = mutable.value
            val name = current.name.trim()
            if (name.isEmpty()) return
            val scopes = current.selection.scopes()
            if (scopes.isEmpty()) {
                mutable.value = current.copy(scopesMissing = true)
                return
            }
            run {
                val minted = tokens.create(name, scopes)
                mutable.value =
                    mutable.value.copy(
                        // The listed form of the new token, which is the shape the
                        // list holds: the plaintext lives only in `created`.
                        tokens =
                            mutable.value.tokens +
                                ApiTokenDto(
                                    id = minted.id,
                                    name = minted.name,
                                    scopes = minted.scopes,
                                    createdAt = minted.createdAt,
                                ),
                        created = minted,
                        name = "",
                        selection = ScopeSelection.NONE,
                    )
            }
        }

        /**
         * Closes the one-time view of a new token and forgets its plaintext.
         *
         * There is no way to see it again, which the screen says before it is
         * dismissed; keeping it around "just in case" would be a copy of a
         * credential with nowhere to live.
         */
        fun dismissCreated() {
            mutable.value = mutable.value.copy(created = null)
        }

        fun askDelete(token: ApiTokenDto) {
            if (mutable.value.busy) return
            mutable.value = mutable.value.copy(deleting = token, problem = null)
        }

        fun cancelDelete() {
            mutable.value = mutable.value.copy(deleting = null)
        }

        fun confirmDelete() {
            val token = mutable.value.deleting ?: return
            mutable.value = mutable.value.copy(deleting = null)
            run {
                tokens.delete(token.id)
                // Removed once the server has agreed, never before: a row dropped
                // on a failed call would claim an integration is locked out while
                // it is still working.
                mutable.value = mutable.value.copy(tokens = mutable.value.tokens.filterNot { it.id == token.id })
            }
        }

        fun dismissMessage() {
            mutable.value = mutable.value.copy(problem = null, scopesMissing = false)
        }

        private fun select(selection: ScopeSelection) {
            mutable.value = mutable.value.copy(selection = selection, scopesMissing = false)
        }

        /** Runs one call, and refuses to start a second while the first is out. */
        private fun run(block: suspend () -> Unit) {
            if (mutable.value.busy) return
            mutable.value = mutable.value.copy(busy = true, problem = null, scopesMissing = false)
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
