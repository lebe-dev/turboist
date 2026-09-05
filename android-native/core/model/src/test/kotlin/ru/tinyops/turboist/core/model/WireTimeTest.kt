package ru.tinyops.turboist.core.model

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

/**
 * The wire format is a contract with the server, which emits and accepts exactly
 * `yyyy-MM-ddTHH:mm:ss.SSSZ`. These tests pin both directions of the conversion,
 * because a drift here silently misplaces every due date by an hour or a day.
 */
class WireTimeTest {
    @Test
    fun `the write format spells out the server's timestamp layout field for field`() {
        // The server formats every timestamp with the reference layout
        // "2006-01-02T15:04:05.000Z": four-digit year, zero-padded month and day, a
        // 24-hour clock, exactly three fractional digits, literal `T` and `Z`.
        // Rewriting that layout into the pattern syntax this object uses must land
        // on FORMAT — so if either side is ever loosened (a dropped `Z`, a fourth
        // fractional digit), the mismatch shows up here rather than as timestamps
        // the server rejects.
        val fromServerLayout =
            "2006-01-02T15:04:05.000Z"
                .replace("2006", "uuuu")
                .replace("01", "MM")
                .replace("02", "dd")
                .replace("15", "HH")
                .replace("04", "mm")
                .replace("05", "ss")
                .replace("000", "SSS")
                .replace("T", "'T'")
                .replace("Z", "'Z'")
        assertEquals(fromServerLayout, WireTime.FORMAT)
    }

    @Test
    fun `formats epoch millis with three fractional digits and a literal Z`() {
        assertEquals("1970-01-01T00:00:00.000Z", WireTime.format(0L))
        assertEquals("2024-03-09T12:34:56.789Z", WireTime.format(1_709_987_696_789L))
    }

    @Test
    fun `pads fractional digits instead of trimming them`() {
        assertEquals("1970-01-01T00:00:00.007Z", WireTime.format(7L))
        assertEquals("1970-01-01T00:00:00.070Z", WireTime.format(70L))
    }

    @Test
    fun `formats instants before the epoch`() {
        assertEquals("1969-12-31T23:59:59.999Z", WireTime.format(-1L))
    }

    @Test
    fun `parses the canonical form back to the same millis`() {
        assertEquals(0L, WireTime.parse("1970-01-01T00:00:00.000Z"))
        assertEquals(1_709_987_696_789L, WireTime.parse("2024-03-09T12:34:56.789Z"))
    }

    @Test
    fun `round-trips every sample both ways`() {
        val samples = listOf(0L, 1L, -1L, 1_709_987_696_789L, 4_102_444_800_000L)
        for (millis in samples) {
            assertEquals(millis, WireTime.parse(WireTime.format(millis)))
        }
        val texts = listOf("2000-01-01T00:00:00.000Z", "2035-12-31T23:59:59.999Z")
        for (text in texts) {
            assertEquals(text, WireTime.format(WireTime.parse(text)))
        }
    }

    @Test
    fun `parsing tolerates shapes the server does not emit but ISO-8601 allows`() {
        // Older rows and hand-written payloads may carry a different fractional
        // precision or a numeric offset; reading must not fail on them.
        assertEquals(0L, WireTime.parse("1970-01-01T00:00:00Z"))
        assertEquals(1_709_987_696_780L, WireTime.parse("2024-03-09T12:34:56.78Z"))
        assertEquals(1_709_987_696_789L, WireTime.parse("2024-03-09T15:34:56.789+03:00"))
    }

    @Test
    fun `sub-millisecond precision is truncated, never rounded up`() {
        assertEquals(1_709_987_696_789L, WireTime.parse("2024-03-09T12:34:56.789999Z"))
    }

    @Test
    fun `parseOrNull answers null for absent and blank input`() {
        assertNull(WireTime.parseOrNull(null))
        assertNull(WireTime.parseOrNull(""))
        assertNull(WireTime.parseOrNull("   "))
    }

    @Test
    fun `parseOrNull answers null for input that is not a timestamp`() {
        assertNull(WireTime.parseOrNull("yesterday"))
        assertNull(WireTime.parseOrNull("2024-03-09"))
    }

    @Test
    fun `parse rejects input that is not a timestamp`() {
        assertFailsWith<IllegalArgumentException> { WireTime.parse("yesterday") }
    }

    @Test
    fun `formatOrNull mirrors parseOrNull for absent values`() {
        assertNull(WireTime.formatOrNull(null))
        assertEquals("1970-01-01T00:00:00.000Z", WireTime.formatOrNull(0L))
    }
}
