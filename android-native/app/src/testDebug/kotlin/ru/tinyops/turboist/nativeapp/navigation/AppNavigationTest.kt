package ru.tinyops.turboist.nativeapp.navigation

import android.app.Application
import android.content.Intent
import android.net.Uri
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.performClick
import androidx.navigation.NavDestination.Companion.hasRoute
import androidx.navigation.NavHostController
import androidx.navigation.compose.rememberNavController
import kotlinx.coroutines.flow.MutableSharedFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.ui.SettingsNavigationRows
import ru.tinyops.turboist.nativeapp.shell.AppShell
import ru.tinyops.turboist.nativeapp.shell.DrawerCounts
import ru.tinyops.turboist.nativeapp.shell.StubScreen
import ru.tinyops.turboist.nativeapp.tasks.TaskAddress
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.reflect.KClass
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The shell composed for real, on the JVM, and walked the way a user walks it.
 *
 * The drawer is the only way into most of the app, so "the destination list, the
 * navigation graph and the titles agree" is not something the parts can be asked
 * about one at a time — a catalog entry pointing at a route nobody registered
 * reads perfectly and navigates nowhere. Everything below therefore asserts
 * against the graph the composition actually built rather than against a list
 * written out by hand, which is what keeps a destination added tomorrow from
 * passing silently.
 *
 * The window is pinned narrower than the width at which the drawer becomes
 * permanent, because it is the phone layout — menu button, temporary drawer —
 * that has a way to get things wrong. The process is given a bare application
 * object rather than the product's own: nothing here needs the dependency graph
 * or the stored session, and starting them would make a test about navigation
 * depend on everything the app does at launch.
 *
 * It sits in the debug test sources on purpose. The harness launches into an
 * activity contributed by a manifest that exists only for testing, and that
 * manifest has no business being merged into a build that ships.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class AppNavigationTest {
    @get:Rule
    val compose = createComposeRule()

    // Replaying, so a link handed to the shell is delivered whether or not the
    // collector happened to be listening at that instant. Nothing about the
    // assertion depends on that race, and neither should the test.
    private val newIntents = MutableSharedFlow<Intent>(replay = 1)

    private lateinit var navController: NavHostController

    /**
     * Stand-ins for the screens that read the replica.
     *
     * This walk is about the graph, not about what the destinations show, and a
     * real list screen would drag a database, a sync engine and the dependency
     * graph into a test that asserts none of them. The titles below are the same
     * ones the drawer entries carry, which is all the walk looks at.
     */
    private val placeholderScreens =
        AppScreens(
            today = { StubScreen(R.string.nav_today) },
            tomorrow = { StubScreen(R.string.nav_tomorrow) },
            week = { StubScreen(R.string.nav_week) },
            nextWeek = { StubScreen(R.string.nav_nextWeek) },
            inbox = { StubScreen(R.string.nav_inbox) },
            completed = { StubScreen(R.string.nav_completed) },
            search = { StubScreen(R.string.nav_search) },
            // The projects screen reads the replica for the whole workspace, so
            // it stands in for itself too. The two screens it leads to are not
            // in the drawer and are never composed by this walk.
            projects = { _, _ -> StubScreen(R.string.nav_projects) },
            // The daily plan reads the replica for every project standing in it,
            // so it stands in for itself here; nothing in this walk opens one.
            troiki = { _, _ -> StubScreen(R.string.nav_troiki) },
            // The label report counts every tagging in the replica, so it
            // stands in for itself; the label screen it leads to is not in the
            // drawer and is never composed by this walk.
            labels = { StubScreen(R.string.nav_labels) },
            // The jump pair is read from a store of the device's own, so the
            // control that draws it stands in for itself; this walk never uses it.
            harpoon = { _, _ -> },
            // The capture surface reads the replica for the places a task can go,
            // so it stands in for itself here. Nothing in this walk opens it.
            quickAdd = { _ -> },
            // Settings is drawn as its real navigation rows and nothing else.
            // Those rows are the only way to three destinations, so substituting
            // them would remove the very wiring the walk is here to check; the
            // controls underneath them read the replica and the device's own
            // preferences, which a walk about the graph has no business starting.
            settings = { destinations -> SettingsNavigationRows(destinations) },
            // What the first of those rows opens. Stood in for, because the
            // passkey screen talks to the server.
            passkeys = { StubScreen(R.string.settings_passkeys_heading) },
            // The other destination settings leads to. Stood in for as well: the
            // templates screen reads the replica, and the walk is about the row
            // that opens it rather than about what it draws.
            templates = { StubScreen(R.string.settings_templates_heading) },
            // The third: the pile of changes the server refused. Stood in for
            // because it reads the queue, and the walk is about the row.
            unsentChanges = { StubScreen(R.string.offline_unsentTitle) },
            // The three administrative surfaces the remaining rows open. All
            // stood in for: each one talks to the server the moment it appears,
            // and the walk is about the row that leads to it.
            sessions = { StubScreen(R.string.settings_sessions_heading) },
            apiTokens = { StubScreen(R.string.settings_api_heading) },
            twoFactor = { StubScreen(R.string.settings_twofa_heading) },
            // The task screen reads the replica, so it stands in for itself and
            // shows the address the destination handed it — which is the half of
            // a link the walk is actually about.
            task = { address, _, _, _ ->
                StubScreen(
                    R.string.native_dest_task,
                    argument =
                        when (address) {
                            is TaskAddress.Local -> address.taskLocalId
                            is TaskAddress.Server -> address.serverId
                        },
                )
            },
        )

    /** The three routes that live in the authentication graph, not in this one. */
    private val authRoutes: Set<KClass<*>> = setOf(ConnectRoute::class, SetupRoute::class, LoginRoute::class)

    private fun launchShell(
        dailyPlanEnabled: Boolean = true,
        pendingTaskLinks: PendingTaskLinks = PendingTaskLinks(),
    ) {
        compose.setContent {
            navController = rememberNavController()
            TurboistTheme {
                AppShell(
                    counts = DrawerCounts(),
                    newIntents = newIntents,
                    unverifiedSession = false,
                    onSignOut = {},
                    navController = navController,
                    screens = placeholderScreens,
                    dailyPlanEnabled = dailyPlanEnabled,
                    pendingTaskLinks = pendingTaskLinks,
                )
            }
        }
        compose.waitForIdle()
    }

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)

    private fun openDrawer() {
        compose.onNodeWithContentDescription(text(R.string.native_drawer_open)).performClick()
        compose.waitForIdle()
    }

    private fun tapDrawerEntry(destination: DrawerDestination) {
        openDrawer()
        // The label also appears in the title bar; only the drawer entry is
        // clickable, which is what tells the two apart.
        compose.onNode(hasText(text(destination.labelRes)) and hasClickAction()).performClick()
        compose.waitForIdle()
    }

    /** The entries the drawer is showing, in the order it draws them. */
    private fun offeredEntries(dailyPlanEnabled: Boolean = true): List<DrawerDestination> =
        DrawerDestination.grouped(dailyPlanEnabled).flatMap { (_, destinations) -> destinations }

    @Test
    fun `every drawer entry opens the destination it names`() {
        launchShell()

        offeredEntries().forEach { destination ->
            tapDrawerEntry(destination)

            assertTrue(
                navController.currentBackStackEntry?.destination?.hasRoute(destination.route::class) == true,
                "the drawer entry ${destination.name} did not open the destination it names",
            )
            // The title bar is the user's own confirmation that the tap landed,
            // so it is asserted alongside the back stack.
            compose.onAllNodesWithText(text(destination.labelRes)).onFirst().assertIsDisplayed()
        }
    }

    @Test
    fun `settings leads to the account's passkeys`() {
        launchShell()

        tapDrawerEntry(DrawerDestination.Settings)
        // The row, not the heading inside it: the screen's own title is not
        // clickable, and only a row that navigates makes the destination real.
        compose.onNode(hasText(text(R.string.settings_passkeys_heading)) and hasClickAction()).performClick()
        compose.waitForIdle()

        assertTrue(
            navController.currentBackStackEntry?.destination?.hasRoute(PasskeysRoute::class) == true,
            "the settings row did not open the passkeys destination",
        )
        compose.onAllNodesWithText(text(R.string.settings_passkeys_heading)).onFirst().assertIsDisplayed()
    }

    @Test
    fun `settings leads to the reusable templates`() {
        launchShell()

        tapDrawerEntry(DrawerDestination.Settings)
        compose.onNode(hasText(text(R.string.settings_templates_heading)) and hasClickAction()).performClick()
        compose.waitForIdle()

        assertTrue(
            navController.currentBackStackEntry?.destination?.hasRoute(TemplatesRoute::class) == true,
            "the settings row did not open the templates destination",
        )
        compose.onAllNodesWithText(text(R.string.settings_templates_heading)).onFirst().assertIsDisplayed()
    }

    @Test
    fun `settings leads to the changes the server refused`() {
        launchShell()

        tapDrawerEntry(DrawerDestination.Settings)
        compose.onNode(hasText(text(R.string.offline_unsentTitle)) and hasClickAction()).performClick()
        compose.waitForIdle()

        assertTrue(
            navController.currentBackStackEntry?.destination?.hasRoute(UnsentChangesRoute::class) == true,
            "the settings row did not open the unsent-changes destination",
        )
        compose.onAllNodesWithText(text(R.string.offline_unsentTitle)).onFirst().assertIsDisplayed()
    }

    @Test
    fun `settings leads to the sessions that can reach the account`() {
        launchShell()

        tapDrawerEntry(DrawerDestination.Settings)
        compose.onNode(hasText(text(R.string.settings_sessions_heading)) and hasClickAction()).performClick()
        compose.waitForIdle()

        assertTrue(
            navController.currentBackStackEntry?.destination?.hasRoute(SessionsRoute::class) == true,
            "the settings row did not open the sessions destination",
        )
        compose.onAllNodesWithText(text(R.string.settings_sessions_heading)).onFirst().assertIsDisplayed()
    }

    @Test
    fun `settings leads to the API tokens`() {
        launchShell()

        tapDrawerEntry(DrawerDestination.Settings)
        compose.onNode(hasText(text(R.string.settings_api_heading)) and hasClickAction()).performClick()
        compose.waitForIdle()

        assertTrue(
            navController.currentBackStackEntry?.destination?.hasRoute(ApiTokensRoute::class) == true,
            "the settings row did not open the API tokens destination",
        )
        compose.onAllNodesWithText(text(R.string.settings_api_heading)).onFirst().assertIsDisplayed()
    }

    @Test
    fun `settings leads to the second factor`() {
        launchShell()

        tapDrawerEntry(DrawerDestination.Settings)
        compose.onNode(hasText(text(R.string.settings_twofa_heading)) and hasClickAction()).performClick()
        compose.waitForIdle()

        assertTrue(
            navController.currentBackStackEntry?.destination?.hasRoute(TwoFactorRoute::class) == true,
            "the settings row did not open the second-factor destination",
        )
        compose.onAllNodesWithText(text(R.string.settings_twofa_heading)).onFirst().assertIsDisplayed()
    }

    @Test
    fun `re-selecting the destination already on screen does not stack a second copy`() {
        launchShell()

        tapDrawerEntry(DrawerDestination.Inbox)
        val afterFirstTap = navController.currentBackStack.value.size

        tapDrawerEntry(DrawerDestination.Inbox)

        assertEquals(afterFirstTap, navController.currentBackStack.value.size)
    }

    @Test
    fun `every destination in the graph has a title`() {
        launchShell()

        val untitled =
            navController.graph
                .filter { node -> RouteTitles.all.keys.none { node.hasRoute(it) } }
                .mapNotNull { it.route }
        assertEquals(emptyList(), untitled)
    }

    @Test
    fun `no title is left over from a destination the graph no longer has`() {
        launchShell()

        val reachedFromGraph =
            navController.graph
                .flatMap { node -> RouteTitles.all.keys.filter { node.hasRoute(it) } }
                .toSet()
        assertEquals(RouteTitles.all.keys, reachedFromGraph + authRoutes)
    }

    @Test
    fun `the daily plan is not offered to a user who does not keep one`() {
        launchShell(dailyPlanEnabled = false)
        openDrawer()

        compose
            .onNode(hasText(text(DrawerDestination.Troiki.labelRes)) and hasClickAction())
            .assertDoesNotExist()
    }

    @Test
    fun `a task link that had nowhere to land is followed once there is somewhere`() {
        // What a link tapped while nobody was signed in leaves behind: the app
        // shell is the first thing that can answer it, and does so on its own
        // rather than waiting for the intent to be delivered a second time.
        val parked = PendingTaskLinks()
        parked.offer(4242L)

        launchShell(pendingTaskLinks = parked)

        assertTrue(
            navController.currentBackStackEntry?.destination?.hasRoute(TaskLinkRoute::class) == true,
            "a link held across sign-in did not reach the task destination",
        )
        compose.onAllNodesWithText(text(R.string.native_stub_argument, 4242L)).onFirst().assertIsDisplayed()
    }

    @Test
    fun `a task link handed to a running app opens the task screen`() {
        launchShell()

        newIntents.tryEmit(Intent(Intent.ACTION_VIEW, Uri.parse(DeepLinks.task(4242L))))
        compose.waitForIdle()

        assertTrue(
            navController.currentBackStackEntry?.destination?.hasRoute(TaskLinkRoute::class) == true,
            "a task link did not reach the task destination",
        )
        compose.onAllNodesWithText(text(R.string.native_stub_argument, 4242L)).onFirst().assertIsDisplayed()
    }
}
