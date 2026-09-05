package ru.tinyops.turboist.nativeapp.calendar

import kotlinx.serialization.Serializable
import ru.tinyops.turboist.core.model.view.TimeWindow
import ru.tinyops.turboist.core.network.dto.CalendarEventDto
import ru.tinyops.turboist.core.network.dto.CalendarStatusDto

/**
 * The last answer the server gave about the calendar, kept for when it cannot be
 * reached.
 *
 * The entries are stored exactly as they arrived rather than as the values the
 * screens read. A stored copy is then a transcript of a response, which is the
 * only form that stays readable when the shape the screens want changes: a build
 * that adds a field to the displayed value can still read every copy written
 * before it.
 *
 * @property fetchedAt when the answer was received, which is what the screens
 *   show alongside a copy that could not be refreshed.
 * @property from the first instant the copy covers.
 * @property untilExclusive the instant it stops covering.
 */
@Serializable
data class CalendarCacheEntry(
    val fetchedAt: Long,
    val from: Long,
    val untilExclusive: Long,
    val events: List<CalendarEventDto> = emptyList(),
    val status: CalendarStatusDto = CalendarStatusDto(),
) {
    /** Whether the whole of [window] falls inside what this copy covers. */
    fun covers(window: TimeWindow): Boolean = from <= window.from && untilExclusive >= window.untilExclusive
}

/**
 * Where that copy lives.
 *
 * Deliberately not a table of the replica and not reachable from it. The replica
 * is the record of the user's own work, kept in step with the server by a change
 * history and emptied wholesale when that history is replaced; calendar entries
 * belong to a different system entirely, are never sent anywhere, and must
 * survive — or be discarded — on their own terms. Keeping them in a store of
 * their own is what makes both true by construction: a catch-up cannot rewrite
 * them, a full re-copy of the replica cannot drop them, and nothing that walks
 * the replica's tables can ever find them.
 */
interface CalendarCache {
    /** The stored copy, or `null` when there is none or it cannot be read. */
    suspend fun read(): CalendarCacheEntry?

    /** Replaces the stored copy. */
    suspend fun write(entry: CalendarCacheEntry)

    /** Throws the stored copy away. */
    suspend fun clear()
}
