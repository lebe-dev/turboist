package ru.tinyops.turboist.core.model

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SettingsTest {
    @Test
    fun `pinned caps default to ten`() {
        val settings = UserSettings()
        assertEquals(DEFAULT_MAX_PINNED, settings.maxPinnedTasks)
        assertEquals(DEFAULT_MAX_PINNED, settings.maxPinnedProjects)
    }

    @Test
    fun `an out-of-range pinned cap falls back to the default rather than to zero`() {
        assertEquals(DEFAULT_MAX_PINNED, normalizeMaxPinned(0))
        assertEquals(DEFAULT_MAX_PINNED, normalizeMaxPinned(-3))
        assertEquals(DEFAULT_MAX_PINNED, normalizeMaxPinned(51))
        assertEquals(DEFAULT_MAX_PINNED, normalizeMaxPinned(null))
    }

    @Test
    fun `a pinned cap inside the bounds is kept as it is`() {
        assertEquals(MIN_MAX_PINNED, normalizeMaxPinned(MIN_MAX_PINNED))
        assertEquals(MAX_MAX_PINNED, normalizeMaxPinned(MAX_MAX_PINNED))
        assertEquals(7, normalizeMaxPinned(7))
    }

    @Test
    fun `hiding past calendar events is on unless the user turned it off`() {
        assertTrue(UserSettings().calendarHidePastEvents)
    }

    @Test
    fun `an unrestricted today banner has no day part`() {
        assertNull(UserSettings().bannerDayPart)
    }

    @Test
    fun `harpoon slots keep the order they were attached in`() {
        // Slot order carries meaning — the pair is a jump *between* a first and a
        // second slot — so nothing above may treat the list as an unordered set.
        // The two-slot cap itself is the server's to enforce (attaching a third
        // evicts the oldest); this type only has to preserve what it was given.
        val first = HarpoonRef(HarpoonKind.TASK, 1L)
        val second = HarpoonRef(HarpoonKind.PROJECT, 2L)
        val settings = UserSettings(harpoon = listOf(first, second))
        assertEquals(listOf(first, second), settings.harpoon)
    }

    @Test
    fun `a rule matches case-sensitively unless the payload says otherwise`() {
        // The server's flag is a plain boolean with no explicit default, so a rule
        // that arrives without it must land on case-sensitive matching. Flipping
        // this default would quietly widen every stored mask.
        assertFalse(AutoLabelRule(mask = "bug").ignoreCase)
        assertFalse(ProjectSuggestionRule(mask = "bug").ignoreCase)
    }

    @Test
    fun `an installation with no rules configured applies and suggests nothing`() {
        // An empty rule set is the resting state, not a missing one: a decoder that
        // read `null` for either list must still produce a settings object the
        // quick-add path can iterate without a null check.
        val appSettings = AppSettings()
        assertTrue(appSettings.autoLabels.isEmpty())
        assertTrue(appSettings.projectSuggestions.isEmpty())
    }

    @Test
    fun `user state is empty until a context is made active`() {
        assertNull(UserState().activeContextId)
        assertEquals(9L, UserState(activeContextId = 9L).activeContextId)
    }
}
