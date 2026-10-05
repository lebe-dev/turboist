package ru.tinyops.turboist.core.network

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ServerUrlTest {
    @Test
    fun `a bare host is read as https`() {
        assertEquals("https://tasks.example.com/", ServerUrl.normalize("tasks.example.com").toString())
    }

    @Test
    fun `plain http has to be asked for explicitly`() {
        assertEquals("http://192.168.1.10:8080/", ServerUrl.normalize("http://192.168.1.10:8080").toString())
    }

    @Test
    fun `a sub-path address keeps its prefix and gains a trailing slash`() {
        assertEquals("https://example.com/turboist/", ServerUrl.normalize("https://example.com/turboist").toString())
    }

    @Test
    fun `surrounding whitespace is forgiven`() {
        assertEquals("https://example.com/", ServerUrl.normalize("  https://example.com  ").toString())
    }

    @Test
    fun `a non-http scheme names no server`() {
        assertNull(ServerUrl.normalize("ftp://example.com"))
        assertNull(ServerUrl.normalize(""))
        assertNull(ServerUrl.normalize("   "))
    }

    @Test
    fun `an address can be set, replaced and forgotten while the stack keeps running`() {
        val url = ServerUrl()
        assertFalse(url.isConfigured)

        url.set("example.com")
        assertTrue(url.isConfigured)
        assertEquals("https://example.com/", url.value.toString())

        url.set("http://localhost:18080")
        assertEquals("http://localhost:18080/", url.value.toString())

        url.clear()
        assertFalse(url.isConfigured)
    }

    @Test
    fun `an unusable address is refused instead of being stored`() {
        val url = ServerUrl()
        assertFailsWith<IllegalArgumentException> { url.set("not a url at all") }
        assertFalse(url.isConfigured)
    }
}
