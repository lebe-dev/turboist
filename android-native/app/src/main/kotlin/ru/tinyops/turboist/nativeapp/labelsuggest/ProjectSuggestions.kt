package ru.tinyops.turboist.nativeapp.labelsuggest

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import ru.tinyops.turboist.core.model.labelsuggest.ProjectSuggestionRecipe

/**
 * One question: which projects suit this title.
 *
 * @property exclude projects never to offer — the ones a title rule already offers.
 * @property limit the most projects returned. A caller that filters the answer
 *   further asks for a few more than it shows.
 */
data class ProjectSuggestionRequest(
    val title: String,
    val exclude: Set<Long> = emptySet(),
    val limit: Int = ProjectSuggestionRecipe.MAX_SUGGESTIONS,
)

/** Projects suggested as the place for a task, worked out on this device. */
interface ProjectSuggestions {
    /** True while project suggestions are on, the model is here and there is an open project to suggest. */
    val available: StateFlow<Boolean>

    /** The open projects worth offering for [request], best first, by local id; empty when there are none. */
    suspend fun suggest(request: ProjectSuggestionRequest): List<Long>

    /** For screens and tests that run without suggestions. */
    object None : ProjectSuggestions {
        override val available: StateFlow<Boolean> = MutableStateFlow(false).asStateFlow()

        override suspend fun suggest(request: ProjectSuggestionRequest): List<Long> = emptyList()
    }
}
