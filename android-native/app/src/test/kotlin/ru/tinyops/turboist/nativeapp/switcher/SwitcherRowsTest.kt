package ru.tinyops.turboist.nativeapp.switcher

import org.junit.Test
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.nativeapp.tasks.task
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class SwitcherRowsTest {
    private fun taskRow(id: Long) = SwitcherRow.TaskRow(task(id, title = "task $id"), projectTitle = null)

    private fun projectRow(id: Long) =
        SwitcherRow.ProjectRow(
            Project(localId = id, contextLocalId = 1, title = "project $id", createdAt = 0, updatedAt = 0),
        )

    private fun List<SwitcherRow>.keys() = map { "${it.ref.kind.code}:${it.ref.localId}" }

    @Test
    fun `nothing typed shows the recents alone, most recent first, marked recent`() {
        val rows = switcherRows("", listOf(taskRow(1), projectRow(2)), emptyList(), emptyList())
        assertEquals(listOf("t:1", "p:2"), rows.keys())
        assertTrue(rows.all { it.recent })
    }

    @Test
    fun `matching recents lead, then everything else by rank with projects first within a rank`() {
        val rows =
            switcherRows(
                typed = "garden",
                recents = listOf(taskRow(10), projectRow(20), taskRow(11)),
                tasks = listOf(RankedRow(taskRow(31), 0), RankedRow(taskRow(10), 0), RankedRow(taskRow(30), 1)),
                projects = listOf(RankedRow(projectRow(20), 0), RankedRow(projectRow(40), 1)),
            )
        assertEquals(listOf("t:10", "p:20", "t:31", "p:40", "t:30"), rows.keys())
        assertEquals(listOf(true, true, false, false, false), rows.map { it.recent })
    }

    @Test
    fun `a recent that does not match is left out`() {
        val rows = switcherRows("mom", listOf(taskRow(10)), listOf(RankedRow(taskRow(11), 0)), emptyList())
        assertEquals(listOf("t:11"), rows.keys())
    }
}
