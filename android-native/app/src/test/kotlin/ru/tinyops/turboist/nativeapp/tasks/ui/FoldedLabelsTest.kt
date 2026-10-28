package ru.tinyops.turboist.nativeapp.tasks.ui

import ru.tinyops.turboist.core.model.Label
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * Which labels a folded picker shows before "more".
 *
 * The folded picker exists so a workspace with twenty labels does not open the
 * capture sheet on a wall of chips. What it must never do is hide a label the
 * task already carries, or one the user reached for a moment ago.
 */
class FoldedLabelsTest {
    private val known = (1L..10L).map { label(it) }

    @Test
    fun `with nothing picked and nothing recent, the first few are offered`() {
        val folded = foldedLabels(known, recent = emptyList(), selected = emptyList())

        assertEquals((1L..6L).toList(), folded.map { it.localId })
    }

    @Test
    fun `labels the task carries lead, however far down the list they are`() {
        val folded = foldedLabels(known, recent = emptyList(), selected = listOf("l9"))

        assertEquals(listOf(9L, 1L, 2L, 3L, 4L, 5L), folded.map { it.localId })
    }

    @Test
    fun `recently used labels follow the picked ones and are not repeated`() {
        val folded = foldedLabels(known, recent = listOf(label(8), label(9)), selected = listOf("l9"))

        assertEquals(listOf(9L, 8L, 1L, 2L, 3L, 4L), folded.map { it.localId })
    }

    @Test
    fun `every picked label is shown even when there are more of them than the fold holds`() {
        val picked = known.map { it.name }

        val folded = foldedLabels(known, recent = emptyList(), selected = picked)

        assertEquals(known.size, folded.size)
    }

    private fun label(id: Long): Label = Label(localId = id, serverId = id, name = "l$id", createdAt = 0, updatedAt = 0)
}
