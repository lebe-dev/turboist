package ru.tinyops.turboist.nativeapp.search

import org.junit.Test
import kotlin.test.assertEquals

/**
 * The rule the recent-search list follows.
 *
 * Checked as a function rather than through a file on disk: what has to stay
 * true is the ordering, the de-duplication and the cap, and none of those are
 * facts about storage.
 */
class RecentSearchesTest {
    @Test
    fun `a query is remembered in the shape it will be shown in`() {
        assertEquals("renew passport", normalisedQuery("  renew   passport "))
        assertEquals("renew passport", normalisedQuery("renew\npassport"))
        assertEquals("", normalisedQuery("   "))
    }

    @Test
    fun `the newest search leads the list`() {
        val list = withRecentSearch(withRecentSearch(emptyList(), "passport"), "flights")

        assertEquals(listOf("flights", "passport"), list)
    }

    @Test
    fun `searching for the same thing again moves it up rather than repeating it`() {
        val list = withRecentSearch(listOf("flights", "passport"), "passport")

        assertEquals(listOf("passport", "flights"), list)
    }

    @Test
    fun `the same words in different case are the same search`() {
        val list = withRecentSearch(listOf("passport"), "PASSPORT")

        assertEquals(listOf("PASSPORT"), list)
    }

    @Test
    fun `the list stays short enough to read at a glance`() {
        val list = (1..20).fold(emptyList<String>()) { acc, i -> withRecentSearch(acc, "query $i") }

        assertEquals(RECENT_SEARCH_LIMIT, list.size)
        assertEquals("query 20", list.first())
    }

    @Test
    fun `nothing worth remembering is not remembered`() {
        assertEquals(listOf("passport"), withRecentSearch(listOf("passport"), "   "))
    }
}
