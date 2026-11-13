package ru.tinyops.turboist.nativeapp.labelsuggest

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import ru.tinyops.turboist.core.database.TurboistDatabase
import javax.inject.Inject

/** What the label-suggestion rows in settings show. */
data class LabelSuggestionSettings(
    val supported: Boolean = true,
    val enabled: Boolean = false,
    val model: ModelState = ModelState.Absent(),
    val progress: IndexProgress = IndexProgress(),
    val modelBytes: Long = EmbeddingModel.SIZE_BYTES,
)

@HiltViewModel
class LabelSuggestionSettingsViewModel
    @Inject
    constructor(
        private val preference: LabelSuggestionPreference,
        private val model: ModelStore,
        private val suggestions: OnDeviceLabelSuggestions,
        private val embedder: LiteRtEmbedder,
        private val database: TurboistDatabase,
    ) : ViewModel() {
        val state: StateFlow<LabelSuggestionSettings> =
            combine(preference.enabled, model.state, suggestions.progress) { enabled, state, progress ->
                LabelSuggestionSettings(
                    supported = LabelSuggestionSupport.supported,
                    enabled = enabled,
                    model = state,
                    progress = progress,
                )
            }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), LabelSuggestionSettings())

        /**
         * Turning suggestions on fetches the model straight away — the switch says
         * how large it is — and turning them off stops a download without losing
         * what already arrived, so turning them back on carries on.
         */
        fun setEnabled(enabled: Boolean) {
            viewModelScope.launch {
                preference.setEnabled(enabled)
                if (enabled) model.download() else model.cancel()
            }
        }

        fun retry() = model.download()

        /** Removes the model and everything computed with it, and turns suggestions off. */
        fun deleteModel() {
            viewModelScope.launch {
                preference.setEnabled(false)
                embedder.release()
                model.delete()
                withContext(Dispatchers.IO) { database.textEmbeddings().clearEmbeddings() }
            }
        }

        private companion object {
            const val STOP_TIMEOUT_MILLIS = 5_000L
        }
    }
