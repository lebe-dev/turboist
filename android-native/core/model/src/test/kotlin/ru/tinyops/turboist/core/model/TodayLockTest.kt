package ru.tinyops.turboist.core.model

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** The Today focus lock, answered from plain facts. Times are epoch milliseconds. */
class TodayLockTest {
    private val now = 1_000L
    private val until = 5_000L
    private val today = 2_000L
    private val overdue = 500L
    private val tomorrow = 6_000L
    private val focus = 1L
    private val other = 2L

    private fun refuses(
        before: TodayLock.Placement?,
        after: TodayLock.Placement,
        lockUntil: Long? = until,
        todayProjects: Set<Long> = setOf(focus),
    ) = TodayLock.refuses(lockUntil, now, before, after) { todayProjects }

    private fun open(
        project: Long?,
        due: Long?,
    ) = TodayLock.Placement(project, due, open = true)

    @Test
    fun `a lock runs until its end and not a moment longer`() {
        assertFalse(TodayLock.isActive(null, now))
        assertTrue(TodayLock.isActive(until, now))
        assertFalse(TodayLock.isActive(until, until))
        assertFalse(TodayLock.isActive(until, until + 1))
    }

    @Test
    fun `without a lock nothing is refused`() {
        assertFalse(refuses(null, open(other, today), lockUntil = null))
        assertFalse(refuses(null, open(null, today), lockUntil = null))
        assertFalse(refuses(null, open(other, today), lockUntil = now - 1))
    }

    @Test
    fun `today in one of today's projects is allowed`() {
        assertFalse(refuses(null, open(focus, today)))
    }

    @Test
    fun `today in another project is refused`() {
        assertTrue(refuses(null, open(other, today)))
    }

    @Test
    fun `an overdue date counts as today`() {
        assertTrue(refuses(null, open(other, overdue)))
    }

    @Test
    fun `the inbox and a bare context are never taken into a locked today`() {
        assertTrue(refuses(null, open(null, today), todayProjects = setOf(focus)))
    }

    @Test
    fun `another day or no date is always allowed`() {
        assertFalse(refuses(null, open(other, tomorrow)))
        assertFalse(refuses(null, open(null, null)))
        assertFalse(refuses(null, open(other, until)), "midnight itself is already tomorrow")
    }

    @Test
    fun `a closed task is never on today`() {
        assertFalse(refuses(null, TodayLock.Placement(other, today, open = false)))
    }

    @Test
    fun `a task already on today in the same project may be edited`() {
        assertFalse(refuses(open(other, today), open(other, today + 100), todayProjects = emptySet()))
        assertFalse(refuses(open(null, today), open(null, today), todayProjects = emptySet()))
    }

    @Test
    fun `moving a today task into another project is refused`() {
        assertTrue(refuses(open(focus, today), open(other, today)))
    }

    @Test
    fun `reopening a today task of another project is refused`() {
        assertTrue(refuses(TodayLock.Placement(other, today, open = false), open(other, today)))
    }

    @Test
    fun `the set of projects is only read when the answer depends on it`() {
        var reads = 0
        val read = {
            reads++
            setOf(focus)
        }
        TodayLock.refuses(null, now, null, open(other, today), read)
        TodayLock.refuses(until, now, null, open(other, tomorrow), read)
        TodayLock.refuses(until, now, null, open(null, today), read)
        assertEquals(0, reads)
        TodayLock.refuses(until, now, null, open(other, today), read)
        assertEquals(1, reads)
    }
}
