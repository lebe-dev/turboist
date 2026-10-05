package ru.tinyops.turboist.core.network.dto

import kotlinx.serialization.Serializable

/**
 * Wire shapes of the external calendar, which is read and never written.
 *
 * These payloads are the one thing the app stores outside the replica, so unlike
 * the replicated DTOs they are serialized back out again — the stored copy is
 * literally the answer the server last gave. That makes every default here load
 * bearing twice over: it lets a payload from an older server decode, and it lets
 * a copy written by an older build of this app decode too.
 */
@Serializable
data class CalendarEventDto(
    val id: String = "",
    val sourceId: Long = 0,
    val sourceName: String = "",
    val sourceColor: String = "",
    val title: String = "",
    val location: String = "",
    val start: String = "",
    val end: String = "",
    val startDate: String? = null,
    val endDate: String? = null,
    val allDay: Boolean = false,
    val htmlLink: String = "",
)

@Serializable
data class CalendarEventsDto(
    val items: List<CalendarEventDto> = emptyList(),
)

@Serializable
data class CalendarAccountDto(
    val id: Long = 0,
    val provider: String = "",
    val email: String = "",
    val displayName: String = "",
)

@Serializable
data class CalendarSourceDto(
    val id: Long = 0,
    val accountId: Long = 0,
    val summary: String = "",
    val color: String = "",
    val selected: Boolean = false,
    val isPrimary: Boolean = false,
)

/** The integration's state: whether it is on, and what it is pointed at. */
@Serializable
data class CalendarStatusDto(
    val enabled: Boolean = false,
    val googleConfigured: Boolean = false,
    val accounts: List<CalendarAccountDto> = emptyList(),
    val sources: List<CalendarSourceDto> = emptyList(),
)
