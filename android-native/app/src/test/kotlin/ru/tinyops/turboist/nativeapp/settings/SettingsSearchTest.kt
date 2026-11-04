package ru.tinyops.turboist.nativeapp.settings

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Finding a setting by what the user remembers about it.
 *
 * The match is over words the user can see — the setting's own name and the
 * section it sits in — plus a few words people reach for that the name does not
 * use. Each typed word has to be found somewhere, so a second word narrows the
 * list rather than widening it.
 */
class SettingsSearchTest {
    private val theme = SettingsSearchEntry("Theme", SettingsSection.GENERAL, "General", "dark light")
    private val language = SettingsSearchEntry("Language", SettingsSection.GENERAL, "General")
    private val server = SettingsSearchEntry("Server", SettingsSection.DEVICE, "This device", "address")
    private val entries = listOf(theme, language, server)

    @Test
    fun `nothing typed finds nothing, so the sections stay on screen`() {
        assertTrue(matchSettings("   ", entries).isEmpty())
    }

    @Test
    fun `a setting is found by its name regardless of case`() {
        assertEquals(listOf(theme), matchSettings("THE", entries))
    }

    @Test
    fun `a setting is found by a word its name does not use`() {
        assertEquals(listOf(theme), matchSettings("dark", entries))
    }

    @Test
    fun `a setting is found by the section it lives in`() {
        assertEquals(listOf(server), matchSettings("device", entries))
    }

    @Test
    fun `every typed word has to match, so a second word narrows the list`() {
        assertEquals(listOf(theme, language), matchSettings("general", entries))
        assertEquals(listOf(language), matchSettings("general lang", entries))
    }
}
