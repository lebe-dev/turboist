package ru.tinyops.turboist.nativeapp.shell

import ru.tinyops.turboist.nativeapp.navigation.DrawerDestination
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class DrawerBadgeTest {
    @Test
    fun `unknown counters draw no badge at all`() {
        val counts = DrawerCounts()
        DrawerDestination.entries.forEach { destination ->
            assertNull(counts.badgeFor(destination), "expected no badge for $destination")
        }
    }

    @Test
    fun `an inbox with tasks in it shows how many`() {
        assertEquals(DrawerBadge.Count(7), DrawerCounts(inbox = 7).badgeFor(DrawerDestination.Inbox))
    }

    @Test
    fun `an empty inbox stays silent`() {
        // Zero is the desired state and needs no marker.
        assertNull(DrawerCounts(inbox = 0).badgeFor(DrawerDestination.Inbox))
    }

    @Test
    fun `the week views show the plan against its cap`() {
        val counts = DrawerCounts(weekPlanned = 0, weekLimit = 12)
        assertEquals(DrawerBadge.Ratio(0, 12), counts.badgeFor(DrawerDestination.Week))
        assertEquals(DrawerBadge.Ratio(0, 12), counts.badgeFor(DrawerDestination.NextWeek))
    }

    @Test
    fun `a plan count without a cap is not shown as a ratio`() {
        assertNull(DrawerCounts(weekPlanned = 4).badgeFor(DrawerDestination.Week))
        assertNull(DrawerCounts(weekLimit = 12).badgeFor(DrawerDestination.Week))
    }

    @Test
    fun `destinations without a counter never carry one`() {
        val counts = DrawerCounts(inbox = 3, weekPlanned = 4, weekLimit = 12)
        listOf(
            DrawerDestination.Today,
            DrawerDestination.Tomorrow,
            DrawerDestination.Projects,
            DrawerDestination.Labels,
            DrawerDestination.Completed,
            DrawerDestination.Troiki,
            DrawerDestination.Search,
            DrawerDestination.Settings,
        ).forEach { destination ->
            assertNull(counts.badgeFor(destination), "expected no badge for $destination")
        }
    }

    @Test
    fun `changes the server refused are marked on the way to the screen that lists them`() {
        // The refusal itself is long past and its strip has gone; the mark is
        // how the pile stays findable without a permanent row about it.
        assertEquals(DrawerBadge.Count(2), DrawerCounts(unsentChanges = 2).badgeFor(DrawerDestination.Settings))
    }

    @Test
    fun `a device with nothing refused marks nothing`() {
        assertNull(DrawerCounts(unsentChanges = 0).badgeFor(DrawerDestination.Settings))
    }

    @Test
    fun `the stand-in source publishes nothing rather than invented numbers`() {
        assertEquals(DrawerCounts(), EmptyDrawerCountsSource().counts.value)
    }
}
