package ru.tinyops.turboist.nativeapp.quickadd

import org.junit.Test
import ru.tinyops.turboist.core.model.Project
import kotlin.test.assertEquals
import kotlin.test.assertFalse

/**
 * The device's memory of where it files things.
 *
 * Two rules to keep: what is remembered, and what is shown. They are separate on
 * purpose — the memory is deeper than the row, so a picker that has narrowed its
 * list still has enough history left to fill one.
 */
class RecentProjectsTest {
    @Test
    fun `the most recent project leads and is never listed twice`() {
        val after = withRecentProject(withRecentProject(listOf(3L, 1L), 1), 2)

        assertEquals(listOf(2L, 1L, 3L), after)
    }

    @Test
    fun `visiting the project already at the front changes nothing`() {
        assertEquals(listOf(4L, 9L), withRecentProject(listOf(4L, 9L), 4))
    }

    @Test
    fun `the memory stops at its limit, dropping the least recent`() {
        val filled =
            (1L..RECENT_PROJECTS_MEMORY.toLong()).fold(emptyList<Long>()) { acc, id ->
                withRecentProject(acc, id)
            }

        val after = withRecentProject(filled, 99)

        assertEquals(RECENT_PROJECTS_MEMORY, after.size)
        assertEquals(99L, after.first())
        assertFalse(1L in after, "the least recent project survived the cap")
    }

    private val projects = (1L..6L).map { project(it, "Project $it") }

    private fun pick(
        order: List<Long>,
        candidates: List<Project> = projects,
    ) = pickRecent(order, candidates, Project::localId).map { it.localId }

    @Test
    fun `the row leads with the remembered order, most recent first`() {
        assertEquals(listOf(5L, 2L, 6L), pick(listOf(5, 2, 6, 1)))
    }

    @Test
    fun `a project the caller has filtered out never comes back through the row`() {
        val narrowed = projects.filter { it.localId != 2L }

        assertEquals(listOf(5L, 6L, 1L), pick(listOf(5, 2, 6, 1), narrowed))
    }

    /**
     * A row that repeats the whole picker is noise, and it would offer the same
     * project twice — once at the top and once in the list it was lifted from.
     */
    @Test
    fun `a list that already fits in the row gets no row`() {
        val three = projects.take(RECENT_PROJECTS_SHOWN)

        assertEquals(emptyList(), pick(listOf(1, 2, 3), three))
    }

    @Test
    fun `a memory of projects that are all gone leaves the row empty`() {
        assertEquals(emptyList(), pick(listOf(101, 102)))
    }
}
