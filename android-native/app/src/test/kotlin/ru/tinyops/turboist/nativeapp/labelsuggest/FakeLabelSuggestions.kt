package ru.tinyops.turboist.nativeapp.labelsuggest

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/** Answers every question with [answer] and remembers what it was asked. */
class FakeLabelSuggestions : LabelSuggestions {
    var answer: List<Long> = emptyList()
    val asked = mutableListOf<SuggestionRequest>()

    override val available: StateFlow<Boolean> = MutableStateFlow(true)

    override suspend fun suggest(request: SuggestionRequest): List<Long> {
        asked += request
        return answer.filter { it !in request.exclude }.take(request.limit)
    }
}
