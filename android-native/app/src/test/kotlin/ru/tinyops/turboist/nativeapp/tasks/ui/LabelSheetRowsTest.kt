package ru.tinyops.turboist.nativeapp.tasks.ui

import ru.tinyops.turboist.core.model.Label
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * How the label picker splits its labels between the suggested row, the
 * recent row and the rest: every label exactly once, the model's guesses
 * first unless the user is searching.
 */
class LabelSheetRowsTest {
    /** More labels than the recent row holds, so the row is not skipped as redundant. */
    private val labels = (1L..10L).map { Label(localId = it, name = "label $it", createdAt = 0, updatedAt = 0) }

    private fun ids(list: List<Label>) = list.map { it.localId }

    @Test
    fun `suggestions lead, recent labels follow, and nothing is offered twice`() {
        val rows = labelSheetRows(labels, searching = false, recentOrder = listOf(3, 4, 5), suggestedIds = listOf(4, 9))

        assertEquals(listOf(4L, 9L), ids(rows.suggested))
        assertEquals(listOf(3L, 5L), ids(rows.recent))
        assertEquals(listOf(1L, 2L, 6L, 7L, 8L, 10L), ids(rows.rest))
    }

    @Test
    fun `a search hides the suggestions and gives their labels back to the list`() {
        val rows = labelSheetRows(labels, searching = true, recentOrder = emptyList(), suggestedIds = listOf(4, 9))

        assertEquals(emptyList(), rows.suggested)
        assertEquals(ids(labels), ids(rows.rest))
    }

    @Test
    fun `a suggested label that is gone from the workspace is not offered`() {
        val matches = labels.filter { it.localId != 9L }

        val rows = labelSheetRows(matches, searching = false, recentOrder = emptyList(), suggestedIds = listOf(9, 2))

        assertEquals(listOf(2L), ids(rows.suggested))
    }
}
