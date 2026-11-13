package ru.tinyops.turboist.nativeapp.labelsuggest

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import ru.tinyops.turboist.core.database.TurboistDatabase
import javax.inject.Inject

/** What the suggestion rows in settings show. */
data class SuggestionSettings(
    val supported: Boolean = true,
    val labelsEnabled: Boolean = false,
    val projectsEnabled: Boolean = false,
    val model: ModelState = ModelState.Absent(),
    val progress: IndexProgress = IndexProgress(),
    val modelBytes: Long = EmbeddingModel.SIZE_BYTES,
) {
    /** True while either kind of suggestion is on, which is what needs the model. */
    val anyEnabled: Boolean get() = labelsEnabled || projectsEnabled
}

@HiltViewModel
class SuggestionSettingsViewModel
    @Inject
    constructor(
        private val preference: SuggestionPreference,
        private val model: ModelStore,
        private val suggestions: OnDeviceSuggestions,
        private val embedder: LiteRtEmbedder,
        private val database: TurboistDatabase,
    ) : ViewModel() {
        val state: StateFlow<SuggestionSettings> =
            combine(preference.switches, model.state, suggestions.progress) { switches, state, progress ->
                SuggestionSettings(
                    supported = LabelSuggestionSupport.supported,
                    labelsEnabled = switches.labels,
                    projectsEnabled = switches.projects,
                    model = state,
                    progress = progress,
                )
            }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), SuggestionSettings())

        fun setLabelsEnabled(enabled: Boolean) {
            viewModelScope.launch {
                preference.setLabelsEnabled(enabled)
                followSwitches()
            }
        }

        fun setProjectsEnabled(enabled: Boolean) {
            viewModelScope.launch {
                preference.setProjectsEnabled(enabled)
                followSwitches()
            }
        }

        fun retry() = model.download()

        /** Removes the model and everything computed with it, and turns both kinds of suggestion off. */
        fun deleteModel() {
            viewModelScope.launch {
                preference.disableAll()
                embedder.release()
                model.delete()
                withContext(Dispatchers.IO) { database.textEmbeddings().clearEmbeddings() }
            }
        }

        /**
         * Turning either switch on fetches the model straight away — the switch
         * says how large it is. Turning the last one off stops a download without
         * losing what already arrived, so turning one back on carries on.
         */
        private suspend fun followSwitches() {
            if (preference.switches.first().any) model.download() else model.cancel()
        }

        private companion object {
            const val STOP_TIMEOUT_MILLIS = 5_000L
        }
    }
