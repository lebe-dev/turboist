package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.UserSettings

/**
 * The announcement the day view leads with, or `null` when there is none.
 *
 * The text is the user's own and travels with their preferences, so it is shown
 * verbatim. Three things have to hold for it to appear: it has to be published,
 * it has to say something, and — when it was scoped to a phase of the day — that
 * phase has to be the one happening now. A phase-scoped announcement is
 * therefore invisible outside its phase rather than merely quieter, which is the
 * point of scoping one.
 */
fun todayAnnouncement(
    settings: UserSettings,
    activePart: DayPart?,
): String? {
    if (!settings.bannerPublished) return null
    val text = settings.bannerText.trim()
    if (text.isEmpty()) return null
    val scope = settings.bannerDayPart ?: return text
    return if (scope == activePart) text else null
}
