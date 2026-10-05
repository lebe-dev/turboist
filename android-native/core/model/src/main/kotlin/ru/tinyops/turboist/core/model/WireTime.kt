package ru.tinyops.turboist.core.model

import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException

/**
 * The single conversion between the timestamps the API speaks and the numbers the
 * replica stores.
 *
 * Every timestamp on the wire is UTC with exactly three fractional digits and a
 * literal `Z` — `2024-03-09T12:34:56.789Z`. On the device the same instant is one
 * `Long` of epoch milliseconds, which sorts, compares and indexes without parsing
 * anything.
 *
 * Writing is strict, because the server accepts only its own format. Reading is
 * lenient over ISO-8601 instants — a value with a different fractional precision
 * or a numeric offset still names an unambiguous moment, and refusing it would
 * cost the user a whole page of data over a formatting detail. Anything finer
 * than a millisecond is truncated, never rounded, so a timestamp can never move
 * forward past the moment it names.
 */
object WireTime {
    /** The exact shape the server emits and accepts. */
    const val FORMAT: String = "uuuu-MM-dd'T'HH:mm:ss.SSS'Z'"

    private val formatter: DateTimeFormatter =
        DateTimeFormatter.ofPattern(FORMAT).withZone(ZoneOffset.UTC)

    // Reading goes through the offset form rather than through Instant.parse:
    // it accepts both `Z` and a numeric offset on every platform level the app
    // supports, whereas Instant.parse only learned numeric offsets on newer
    // runtimes and would fail on an older device.
    private val reader: DateTimeFormatter = DateTimeFormatter.ISO_OFFSET_DATE_TIME

    /** Epoch milliseconds to the wire form. */
    fun format(epochMillis: Long): String = formatter.format(Instant.ofEpochMilli(epochMillis))

    /** Epoch milliseconds to the wire form, passing an absent value through. */
    fun formatOrNull(epochMillis: Long?): String? = epochMillis?.let(::format)

    /**
     * The wire form to epoch milliseconds.
     *
     * @throws IllegalArgumentException when the text does not name an instant.
     */
    fun parse(text: String): Long =
        parseOrNull(text) ?: throw IllegalArgumentException("timestamp is not an ISO-8601 instant")

    /** The wire form to epoch milliseconds, answering `null` for absent, blank or malformed input. */
    fun parseOrNull(text: String?): Long? {
        if (text.isNullOrBlank()) return null
        return try {
            OffsetDateTime.parse(text.trim(), reader).toInstant().toEpochMilli()
        } catch (_: DateTimeParseException) {
            null
        } catch (_: ArithmeticException) {
            // An instant so far from the epoch that milliseconds do not fit in a Long.
            null
        }
    }
}
