package ru.tinyops.turboist.nativeapp.labelsuggest

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe

/** Where a suggestion is going to be shown, which decides how sure it has to be. */
enum class SuggestionSurface {
    /** Offered unasked while a title is typed: only labels it is fairly sure of. */
    CAPTURE,

    /** Leading the label picker, where the user is already choosing: the best few, whatever their score. */
    PICKER,
}

/**
 * One question: which labels suit this title.
 *
 * @property projectLocalId the project the task is in or headed for; `null` for the inbox.
 * @property taskLocalId the task being edited, so it does not vote for itself.
 * @property exclude labels never to offer — the ones already on the task or
 *   about to be put there, and the ones the user just took off.
 * @property limit the most labels returned. A caller that filters the answer
 *   further asks for a few more than it shows.
 */
data class SuggestionRequest(
    val title: String,
    val projectLocalId: Long? = null,
    val taskLocalId: Long? = null,
    val exclude: Set<Long> = emptySet(),
    val surface: SuggestionSurface,
    val limit: Int = LabelSuggestionRecipe.MAX_SUGGESTIONS,
)

/** Labels suggested for a task's title, worked out on this device. */
interface LabelSuggestions {
    /** True while suggestions are on, the model is here and there is something to suggest. */
    val available: StateFlow<Boolean>

    /** The labels worth offering for [request], best first, by local id; empty when there are none. */
    suspend fun suggest(request: SuggestionRequest): List<Long>

    /** For screens and tests that run without suggestions. */
    object None : LabelSuggestions {
        override val available: StateFlow<Boolean> = MutableStateFlow(false).asStateFlow()

        override suspend fun suggest(request: SuggestionRequest): List<Long> = emptyList()
    }
}
