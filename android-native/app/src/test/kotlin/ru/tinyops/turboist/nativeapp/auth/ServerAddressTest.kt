package ru.tinyops.turboist.nativeapp.auth

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs

/**
 * What the connect screen accepts.
 *
 * The address typed here is where the account password goes, so the rules are
 * about refusing rather than about parsing: no cleartext, and no guessing.
 */
class ServerAddressTest {
    @Test
    fun `a bare host is read as an encrypted address`() {
        val read = assertIs<ServerAddress.Accepted>(readServerAddress("todo.example.com"))
        assertEquals("https", read.url.scheme)
        assertEquals("todo.example.com", read.url.host)
    }

    @Test
    fun `the address ends in a slash so endpoint paths append to it`() {
        // Without the trailing slash the last segment of the address is replaced
        // by the endpoint path, and every request quietly goes somewhere else.
        val read = assertIs<ServerAddress.Accepted>(readServerAddress("https://example.com/turboist"))
        assertEquals("/turboist/", read.url.encodedPath)
    }

    @Test
    fun `plain http is refused rather than silently downgraded`() {
        assertEquals(ServerAddress.NotEncrypted, readServerAddress("http://todo.example.com"))
    }

    @Test
    fun `a build that permits cleartext may point at a plain http server`() {
        // Only a debug build ever answers true here: the platform permits
        // cleartext where the manifest asked for it, and this rule follows that
        // answer so a developer's local server is reachable and a shipping build
        // still cannot be pointed at one.
        val read = assertIs<ServerAddress.Accepted>(readServerAddress("http://10.0.2.2:18080", allowCleartext = true))
        assertEquals("http", read.url.scheme)
        assertEquals(18080, read.url.port)
    }

    @Test
    fun `a scheme that is neither http nor https is refused whatever the policy`() {
        assertEquals(ServerAddress.Unreadable, readServerAddress("ftp://todo.example.com", allowCleartext = true))
    }

    @Test
    fun `text that names no host is refused`() {
        assertEquals(ServerAddress.Unreadable, readServerAddress("   "))
        assertEquals(ServerAddress.Unreadable, readServerAddress("ftp://todo.example.com"))
    }

    @Test
    fun `surrounding whitespace is forgiven`() {
        // Addresses arrive pasted, and a trailing space is not a typo worth a refusal.
        val read = assertIs<ServerAddress.Accepted>(readServerAddress("  https://todo.example.com  "))
        assertEquals("todo.example.com", read.url.host)
    }
}
