package ru.tinyops.turboist.nativeapp.settings

import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * How a stored device choice is read back.
 *
 * The preferences file outlives the build that wrote it — a phone keeps its
 * choices across updates, and a downgrade is possible too — so an unreadable
 * value has to mean "follow the phone" rather than "stop drawing".
 */
class DeviceOptionsTest {
    @Test
    fun `a stored theme is read back as the choice it names`() {
        assertEquals(ThemeChoice.LIGHT, ThemeChoice.ofStored("LIGHT"))
        assertEquals(ThemeChoice.DARK, ThemeChoice.ofStored("DARK"))
        assertEquals(ThemeChoice.SYSTEM, ThemeChoice.ofStored("SYSTEM"))
    }

    @Test
    fun `a theme this build does not know falls back to following the phone`() {
        assertEquals(ThemeChoice.SYSTEM, ThemeChoice.ofStored(null))
        assertEquals(ThemeChoice.SYSTEM, ThemeChoice.ofStored(""))
        assertEquals(ThemeChoice.SYSTEM, ThemeChoice.ofStored("SEPIA"))
    }

    @Test
    fun `a device nobody has configured syncs and follows the phone`() {
        // The defaults are the product's answer for a phone that has never been
        // asked: an app that quietly stopped catching up until a switch was found
        // would read as broken rather than as frugal.
        val untouched = DeviceOptions()

        assertEquals(ThemeChoice.SYSTEM, untouched.theme)
        assertEquals(true, untouched.syncOnMetered)
    }
}
