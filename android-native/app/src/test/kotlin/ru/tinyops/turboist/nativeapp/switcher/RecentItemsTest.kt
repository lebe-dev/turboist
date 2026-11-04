package ru.tinyops.turboist.nativeapp.switcher

import org.junit.Test
import kotlin.test.assertEquals

class RecentItemsTest {
    private val task1 = RecentRef(SwitcherKind.TASK, 1)
    private val project1 = RecentRef(SwitcherKind.PROJECT, 1)
    private val task2 = RecentRef(SwitcherKind.TASK, 2)

    @Test
    fun `a revisit moves to the front without a duplicate, and a task and a project with one id are different`() {
        var order = emptyList<RecentRef>()
        order = withRecentItem(order, task1)
        order = withRecentItem(order, project1)
        order = withRecentItem(order, task2)
        order = withRecentItem(order, task1)
        assertEquals(listOf(task1, task2, project1), order)
    }

    @Test
    fun `the memory is capped`() {
        var order = emptyList<RecentRef>()
        for (id in 1L..(RECENT_ITEMS_MEMORY + 5L)) order = withRecentItem(order, RecentRef(SwitcherKind.TASK, id))
        assertEquals(RECENT_ITEMS_MEMORY, order.size)
    }

    @Test
    fun `the stored form round-trips and skips what it cannot read`() {
        val order = listOf(task1, project1, task2)
        assertEquals(order, decodeRecentItems(encodeRecentItems(order)))
        assertEquals(listOf(task1), decodeRecentItems("t:1,x:2,p:oops,"))
        assertEquals(emptyList(), decodeRecentItems(null))
    }
}
