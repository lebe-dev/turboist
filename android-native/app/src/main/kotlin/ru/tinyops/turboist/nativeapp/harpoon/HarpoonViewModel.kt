package ru.tinyops.turboist.nativeapp.harpoon

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.sync.write.WriteRefused
import javax.inject.Inject

private const val TAG = "Harpoon"

/** How long the pair stays watched after the control leaves the screen. */
private const val STOP_TIMEOUT_MILLIS = 5_000L

/**
 * The jump pair, as the control in the top bar reads and changes it.
 *
 * It belongs to the shell rather than to a screen, because hopping between two
 * things is only useful from wherever you happen to be.
 */
@HiltViewModel
class HarpoonViewModel
    @Inject
    constructor(
        private val repository: HarpoonRepository,
    ) : ViewModel() {
        /** The pair, oldest first, ready to be drawn. */
        val jumps: StateFlow<List<HarpoonJump>> =
            repository
                .observe()
                .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), emptyList())

        /**
         * Hooks the thing on screen onto the pair, or takes it off again.
         *
         * A refusal leaves the pair as it was, which is the whole of what the
         * control has to say: the only way this is refused is a row that is no
         * longer there, and a jump to it is not something to offer anyway.
         */
        fun toggle(entry: HarpoonEntry) {
            viewModelScope.launch {
                try {
                    if (jumps.value.any { it.entry == entry }) {
                        repository.detach(entry.target, entry.localId)
                    } else {
                        repository.attach(entry.target, entry.localId)
                    }
                } catch (refusal: WriteRefused) {
                    Log.i(TAG, "The jump pair was left unchanged: the write was refused", refusal)
                } catch (failure: RuntimeException) {
                    Log.i(TAG, "The jump pair could not be changed", failure)
                }
            }
        }
    }
