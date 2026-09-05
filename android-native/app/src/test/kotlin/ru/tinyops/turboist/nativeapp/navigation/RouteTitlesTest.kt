package ru.tinyops.turboist.nativeapp.navigation

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The title map on its own. That it covers the navigation graph and carries
 * nothing stale is asserted against the graph the shell actually builds, in the
 * navigation walk — a list of routes repeated here would only ever be as
 * complete as someone remembered to make it.
 */
class RouteTitlesTest {
    @Test
    fun `drawer entries take their title from the drawer catalog`() {
        DrawerDestination.entries.forEach { destination ->
            assertEquals(destination.labelRes, RouteTitles.titleFor(destination.route::class))
        }
    }

    @Test
    fun `the authentication routes have titles of their own`() {
        listOf(ConnectRoute::class, SetupRoute::class, LoginRoute::class).forEach { route ->
            assertNotNull(RouteTitles.titleFor(route), "$route has no title")
        }
    }

    @Test
    fun `every title points at a real resource`() {
        assertTrue(RouteTitles.all.values.all { it != 0 })
    }

    @Test
    fun `an unknown type has no title`() {
        assertNotNull(RouteTitles.titleFor(TaskRoute::class))
        assertNull(RouteTitles.titleFor(String::class))
    }
}
