package ru.tinyops.turboist.nativeapp.tasks.ui

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/**
 * How loudly a row reports that its task keeps being put off.
 *
 * The rules are the web client's: one postponement is a normal day and is not
 * shown, the second is a warning, a third and beyond is an alarm, and a finished
 * task's history no longer competes with the open work around it.
 */
class PostponeToneTest {
    @Test
    fun `a task put off at most once shows nothing`() {
        assertNull(postponeTone(count = 0, completed = false))
        assertNull(postponeTone(count = 1, completed = false))
    }

    @Test
    fun `the second postponement is a warning`() {
        assertEquals(PostponeTone.WARNING, postponeTone(count = 2, completed = false))
    }

    @Test
    fun `a third postponement and beyond is an alarm`() {
        assertEquals(PostponeTone.ALARM, postponeTone(count = 3, completed = false))
        assertEquals(PostponeTone.ALARM, postponeTone(count = 12, completed = false))
    }

    @Test
    fun `a finished task keeps the count but drops the colour`() {
        assertEquals(PostponeTone.MUTED, postponeTone(count = 2, completed = true))
        assertEquals(PostponeTone.MUTED, postponeTone(count = 5, completed = true))
        assertNull(postponeTone(count = 1, completed = true))
    }
}
