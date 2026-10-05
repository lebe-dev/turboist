package ru.tinyops.turboist.nativeapp.account

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * The permission form's rules, which are the server's rules answered early.
 *
 * A token's scopes are fixed for its whole life — there is no call that widens
 * or narrows one — so a selection that the server would refuse is not a
 * recoverable mistake, it is a round trip and a form the user has to fill in
 * again. Every rule below therefore exists to make the refusal impossible to
 * express rather than to report it afterwards.
 */
class TokenScopesTest {
    @Test
    fun `granting write grants reading the same resource`() {
        val selection = ScopeSelection.NONE.withWrite(ScopeResource.TASKS, true)

        // A token that may change work it cannot see is refused outright, so
        // ticking write has to bring read with it.
        assertEquals(listOf("tasks:read", "tasks:write"), selection.scopes())
    }

    @Test
    fun `withdrawing read withdraws writing too`() {
        val selection =
            ScopeSelection.NONE
                .withWrite(ScopeResource.PROJECTS, true)
                .withRead(ScopeResource.PROJECTS, false)

        assertTrue(selection.isEmpty)
        assertEquals(emptyList(), selection.scopes())
    }

    @Test
    fun `read alone is a perfectly ordinary grant`() {
        val selection = ScopeSelection.NONE.withRead(ScopeResource.LABELS, true)

        assertEquals(listOf("labels:read"), selection.scopes())
    }

    @Test
    fun `a read-only resource cannot be granted a write it does not have`() {
        val selection = ScopeSelection.NONE.withWrite(ScopeResource.SEARCH, true)

        // There is no `search:write` on the server, so offering one here would
        // produce a request it rejects for a scope that does not exist.
        assertTrue(selection.isEmpty)
        assertFalse(ScopeResource.SEARCH.writable)
        assertFalse(ScopeResource.CALENDARS.writable)
    }

    @Test
    fun `the wildcard stands alone`() {
        val selection = ScopeSelection.FULL_ACCESS

        assertEquals(listOf("*"), selection.scopes())
        assertTrue(selection.read.isEmpty())
        assertTrue(selection.write.isEmpty())
    }

    @Test
    fun `touching one resource drops the wildcard`() {
        val selection = ScopeSelection.FULL_ACCESS.withRead(ScopeResource.TASKS, true)

        // The server refuses the wildcard alongside anything else, and the two
        // are not the same grant anyway: the wildcard also covers scopes a later
        // server version introduces.
        assertFalse(selection.fullAccess)
        assertEquals(listOf("tasks:read"), selection.scopes())
    }

    @Test
    fun `the read-only preset grants every resource and no write at all`() {
        val scopes = ScopeSelection.READ_ONLY.scopes()

        assertEquals(ScopeResource.entries.map { it.read }, scopes)
        assertTrue(scopes.none { it.endsWith(":${ScopeResource.WRITE}") })
    }

    @Test
    fun `nothing selected produces no scopes, which is a token the server would refuse`() {
        assertTrue(ScopeSelection.NONE.isEmpty)
        assertEquals(emptyList(), ScopeSelection.NONE.scopes())
    }

    @Test
    fun `the scopes are ordered by resource, so one selection always makes one request`() {
        val one =
            ScopeSelection.NONE
                .withWrite(ScopeResource.SETTINGS, true)
                .withRead(ScopeResource.TASKS, true)
        val other =
            ScopeSelection.NONE
                .withRead(ScopeResource.TASKS, true)
                .withWrite(ScopeResource.SETTINGS, true)

        assertEquals(one.scopes(), other.scopes())
        assertEquals(listOf("tasks:read", "settings:read", "settings:write"), one.scopes())
    }

    @Test
    fun `a token carrying the wildcard reads as unrestricted`() {
        assertTrue(listOf("*").grantsFullAccess())
        // Tokens issued before scopes existed come back this way, which is what
        // makes the badge on the list correct for them too.
        assertFalse(listOf("tasks:read").grantsFullAccess())
    }

    @Test
    fun `every resource name is the one the server takes`() {
        // The scope string is the contract. A resource spelled differently here
        // would be refused as unknown rather than quietly narrowed.
        assertEquals(
            listOf(
                "tasks", "projects", "contexts", "labels", "sections",
                "templates", "troiki", "settings", "search", "calendars",
            ),
            ScopeResource.entries.map { it.wire },
        )
    }
}
