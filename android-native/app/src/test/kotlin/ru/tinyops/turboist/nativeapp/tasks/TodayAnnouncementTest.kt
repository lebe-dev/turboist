package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.UserSettings
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** When the message the user wrote for themselves is on the day view, and when it is not. */
class TodayAnnouncementTest {
    private val settings =
        UserSettings(bannerText = "Ship the release", bannerPublished = true)

    @Test
    fun `a published message with no phase shows all day`() {
        assertEquals("Ship the release", todayAnnouncement(settings, activePart = null))
    }

    @Test
    fun `an unpublished message is not shown`() {
        assertNull(todayAnnouncement(settings.copy(bannerPublished = false), activePart = null))
    }

    @Test
    fun `a message that says nothing is not shown`() {
        assertNull(todayAnnouncement(settings.copy(bannerText = "   "), activePart = null))
    }

    @Test
    fun `a message scoped to a phase shows during that phase`() {
        val scoped = settings.copy(bannerDayPart = DayPart.MORNING)

        assertEquals("Ship the release", todayAnnouncement(scoped, activePart = DayPart.MORNING))
    }

    @Test
    fun `a message scoped to a phase is invisible outside it, including when no phase is running`() {
        val scoped = settings.copy(bannerDayPart = DayPart.MORNING)

        assertNull(todayAnnouncement(scoped, activePart = DayPart.EVENING))
        assertNull(todayAnnouncement(scoped, activePart = null))
    }
}
