package ru.tinyops.turboist.nativeapp.shell

import android.content.Intent
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Menu
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalDrawerSheet
import androidx.compose.material3.ModalNavigationDrawer
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.PermanentDrawerSheet
import androidx.compose.material3.PermanentNavigationDrawer
import androidx.compose.material3.Scaffold
import androidx.compose.material3.ScaffoldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.rememberDrawerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.navigation.NavDestination
import androidx.navigation.NavDestination.Companion.hasRoute
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.sync.write.HarpoonTarget
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonEntry
import ru.tinyops.turboist.nativeapp.navigation.AppNavHost
import ru.tinyops.turboist.nativeapp.navigation.AppScreens
import ru.tinyops.turboist.nativeapp.navigation.DrawerDestination
import ru.tinyops.turboist.nativeapp.navigation.LabelsRoute
import ru.tinyops.turboist.nativeapp.navigation.PendingTaskLinks
import ru.tinyops.turboist.nativeapp.navigation.ProjectRoute
import ru.tinyops.turboist.nativeapp.navigation.RouteTitles
import ru.tinyops.turboist.nativeapp.navigation.TaskLinkRoute
import ru.tinyops.turboist.nativeapp.navigation.TaskRoute
import ru.tinyops.turboist.nativeapp.navigation.TemplatesRoute
import ru.tinyops.turboist.nativeapp.sync.SyncStatus

/**
 * Width at which the drawer stops being a temporary overlay and becomes part of
 * the layout. Below it a permanent drawer would eat most of the screen; above
 * it, hiding the destination list behind a tap wastes space that is there.
 */
private val PermanentDrawerBreakpoint = 840.dp

/**
 * How tall the top bar is while the screen is scrolled to the top. Lower than
 * Material's default for a large bar: the title should lead the screen, not
 * take a fifth of it.
 */
private val ExpandedTopBarHeight = 120.dp

/**
 * The places the navigation bar offers on a phone, in its order. They are the
 * four the day is spent between; everything else stays one swipe away in the
 * drawer.
 */
internal val BottomBarDestinations: List<DrawerDestination> =
    listOf(DrawerDestination.Inbox, DrawerDestination.Today, DrawerDestination.Week, DrawerDestination.Projects)

/** Test tag of the navigation bar, which repeats some of the drawer's entries. */
internal const val BOTTOM_BAR_TAG = "bottom-navigation"

/**
 * The signed-in shell: drawer, top bar, and the app's navigation graph.
 *
 * [newIntents] carries links delivered to an already-running app. The graph
 * consumes the launch intent by itself, but a second link arriving while the
 * activity is alive would otherwise be dropped.
 *
 * [unverifiedSession] is true when the app opened on a stored session it could
 * not check with the server. The screens work either way — they read the local
 * replica — but the user is looking at data that has not been reconciled, and
 * that is said rather than hidden.
 *
 * [syncStatus] is the rest of that story: whether a cycle is running, how the
 * last one ended, and how much of the user's own work has not gone out yet. It
 * is drawn in the top bar rather than inside any screen, because it is true of
 * everything below it and because a screen that had to render it would have to
 * know about the engine.
 *
 * [dailyPlanEnabled] is the user's preference for the daily plan, which decides
 * whether the plan is one of the places the drawer offers at all.
 *
 * [pinned] is the shelf of pinned projects and tasks the drawer lists below
 * its destinations.
 *
 * [pendingTaskLinks] holds a link that arrived when there was nothing to show it
 * — during sign-in, or before this tree existed. Following it here is what makes
 * a link tapped on the sign-in screen land on the task once the user is in.
 */
@Composable
fun AppShell(
    counts: DrawerCounts,
    newIntents: Flow<Intent>,
    unverifiedSession: Boolean,
    onSignOut: () -> Unit,
    modifier: Modifier = Modifier,
    syncStatus: SyncStatus = SyncStatus(),
    onRetrySync: () -> Unit = {},
    dailyPlanEnabled: Boolean = false,
    pinned: DrawerPinned = DrawerPinned(),
    pendingTaskLinks: PendingTaskLinks = PendingTaskLinks(),
    navController: NavHostController = rememberNavController(),
    screens: AppScreens = AppScreens(),
) {
    LaunchedEffect(navController) {
        newIntents.collect { navController.handleDeepLink(it) }
    }

    // A link that had nowhere to land is followed as soon as there is somewhere.
    // The graph is built by the host below this, so the first entry it reports is
    // also the first moment a link can go anywhere at all. It is followed as a
    // single top entry, because the graph may have read the very same link out of
    // the launch intent a moment ago and one tap must leave one screen.
    LaunchedEffect(navController) {
        navController.currentBackStackEntryFlow.first()
        pendingTaskLinks.pending().collect { serverId ->
            navController.navigate(TaskLinkRoute(serverId)) { launchSingleTop = true }
            pendingTaskLinks.followed()
        }
    }

    val backStackEntry by navController.currentBackStackEntryAsState()
    val currentDestination = backStackEntry?.destination
    val drawerState = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()

    // What is on screen right now, when it is something that can be hooked onto
    // the jump pair. A project is named the way this device names a row, which is
    // what the pair remembers; a task is too, but it is not here — the task screen
    // draws its own top bar and is handed the control by the graph, because a
    // screen with a bar of its own cannot also be described by this one.
    val harpoonable: HarpoonEntry? =
        backStackEntry?.let { entry ->
            when {
                entry.destination.hasRoute(ProjectRoute::class) ->
                    HarpoonEntry(HarpoonTarget.PROJECT, entry.toRoute<ProjectRoute>().projectLocalId)

                else -> null
            }
        }

    // Jumping is a navigation like any other: the pair holds the ids this device
    // holds its rows under, so an end of it made offline leads somewhere too.
    val jump: (HarpoonEntry) -> Unit = { entry ->
        when (entry.target) {
            HarpoonTarget.TASK -> navController.navigate(TaskRoute(entry.localId))
            HarpoonTarget.PROJECT -> navController.navigate(ProjectRoute(entry.localId))
        }
    }

    // A screen that owns its chrome gets the shell out of the way: no bar of the
    // shell's above its own, and no capture button in the corner it draws in.
    // Only the task screen does — it is the one destination that is about a
    // single thing rather than a list of them, and it needs a back arrow, a pin
    // and an overflow where the shell would put a menu button and a title.
    val screenOwnsChrome =
        currentDestination?.hasRoute(TaskRoute::class) == true ||
            currentDestination?.hasRoute(TaskLinkRoute::class) == true

    // One entry per top-level destination: re-selecting the current one must not
    // stack a second copy, and returning to a destination should find it as it
    // was left rather than reset to the top.
    val open: (DrawerDestination) -> Unit = { destination ->
        navController.navigate(destination.route) {
            popUpTo(navController.graph.findStartDestination().id) { saveState = true }
            launchSingleTop = true
            restoreState = true
        }
    }

    // A pinned entry is a shortcut to a detail screen, not a top-level place: it
    // stacks on whatever is open, like a tap on a row would, so back returns to
    // where the drawer was opened. Tapping the entry already on screen is a no-op.
    val openProject: Long? =
        backStackEntry?.takeIf { it.destination.hasRoute(ProjectRoute::class) }?.toRoute<ProjectRoute>()?.projectLocalId
    val openTask: Long? =
        backStackEntry?.takeIf { it.destination.hasRoute(TaskRoute::class) }?.toRoute<TaskRoute>()?.taskLocalId
    val openPinnedProject: (Long) -> Unit = { localId ->
        if (openProject != localId) navController.navigate(ProjectRoute(localId))
    }
    val openPinnedTask: (Long) -> Unit = { localId ->
        if (openTask != localId) navController.navigate(TaskRoute(localId))
    }

    val chrome = remember { ShellChrome() }

    CompositionLocalProvider(LocalShellChrome provides chrome) {
        BoxWithConstraints(modifier = modifier.fillMaxSize()) {
            if (maxWidth >= PermanentDrawerBreakpoint) {
                PermanentNavigationDrawer(
                    drawerContent = {
                        PermanentDrawerSheet {
                            AppDrawerContent(
                                counts = counts,
                                selected = { currentDestination.isOn(it) },
                                onSelect = open,
                                onSignOut = onSignOut,
                                dailyPlanEnabled = dailyPlanEnabled,
                                pinned = pinned,
                                selectedProject = openProject,
                                selectedTask = openTask,
                                onOpenProject = openPinnedProject,
                                onOpenTask = openPinnedTask,
                            )
                        }
                    },
                ) {
                    ShellScaffold(
                        currentDestination = currentDestination,
                        screenOwnsChrome = screenOwnsChrome,
                        harpoonable = harpoonable,
                        onJump = jump,
                        onOpenDrawer = null,
                        bottomBar = null,
                        chrome = chrome,
                        unverifiedSession = unverifiedSession,
                        syncStatus = syncStatus,
                        onRetrySync = onRetrySync,
                        navController = navController,
                        screens = screens,
                    )
                }
            } else {
                ModalNavigationDrawer(
                    drawerState = drawerState,
                    drawerContent = {
                        ModalDrawerSheet {
                            AppDrawerContent(
                                counts = counts,
                                selected = { currentDestination.isOn(it) },
                                onSelect = { destination ->
                                    open(destination)
                                    scope.launch { drawerState.close() }
                                },
                                onSignOut = onSignOut,
                                dailyPlanEnabled = dailyPlanEnabled,
                                pinned = pinned,
                                selectedProject = openProject,
                                selectedTask = openTask,
                                onOpenProject = { localId ->
                                    openPinnedProject(localId)
                                    scope.launch { drawerState.close() }
                                },
                                onOpenTask = { localId ->
                                    openPinnedTask(localId)
                                    scope.launch { drawerState.close() }
                                },
                            )
                        }
                    },
                ) {
                    ShellScaffold(
                        currentDestination = currentDestination,
                        screenOwnsChrome = screenOwnsChrome,
                        harpoonable = harpoonable,
                        onJump = jump,
                        onOpenDrawer = { scope.launch { drawerState.open() } },
                        bottomBar = {
                            BottomBar(selected = { currentDestination.isOn(it) }, onSelect = open)
                        },
                        chrome = chrome,
                        unverifiedSession = unverifiedSession,
                        syncStatus = syncStatus,
                        onRetrySync = onRetrySync,
                        navController = navController,
                        screens = screens,
                    )
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ShellScaffold(
    currentDestination: NavDestination?,
    screenOwnsChrome: Boolean,
    harpoonable: HarpoonEntry?,
    onJump: (HarpoonEntry) -> Unit,
    onOpenDrawer: (() -> Unit)?,
    bottomBar: (@Composable () -> Unit)?,
    chrome: ShellChrome,
    unverifiedSession: Boolean,
    syncStatus: SyncStatus,
    onRetrySync: () -> Unit,
    navController: NavHostController,
    screens: AppScreens,
) {
    // The title shrinks into the bar as a list scrolls under it and comes back
    // at the top, which is what keeps a large title from costing a list its rows.
    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    // A screen with a create button of its own in the bottom corner keeps that
    // corner: the capture button would be drawn over it and hide it.
    val screenOwnsCorner =
        currentDestination?.hasRoute(LabelsRoute::class) == true ||
            currentDestination?.hasRoute(TemplatesRoute::class) == true
    Scaffold(
        modifier = Modifier.fillMaxSize().nestedScroll(scrollBehavior.nestedScrollConnection),
        // A screen that draws its own bar insets it from the status bar itself;
        // padding the content for it here as well would leave a blank strip
        // the height of the status bar above that bar.
        contentWindowInsets = if (screenOwnsChrome) WindowInsets(0) else ScaffoldDefaults.contentWindowInsets,
        topBar = {
            if (!screenOwnsChrome) {
                LargeTopAppBar(
                    title = {
                        Text(
                            text = stringResource(titleResFor(currentDestination)),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    },
                    navigationIcon = {
                        if (onOpenDrawer != null) {
                            IconButton(onClick = onOpenDrawer) {
                                Icon(
                                    imageVector = Icons.Filled.Menu,
                                    contentDescription = stringResource(R.string.native_drawer_open),
                                )
                            }
                        }
                    },
                    actions = {
                        // How current the screen is sits with the screen's own
                        // controls: it is a fact about what is on screen, and
                        // drawing it in the bar means it never pushes the list.
                        SyncIndicator(
                            status = syncStatus,
                            sessionUnverified = unverifiedSession,
                            onRetry = onRetrySync,
                        )
                        screens.harpoon(harpoonable, onJump)
                    },
                    expandedHeight = ExpandedTopBarHeight,
                    colors =
                        TopAppBarDefaults.topAppBarColors(
                            containerColor = MaterialTheme.colorScheme.surface,
                            scrolledContainerColor = MaterialTheme.colorScheme.surfaceContainer,
                        ),
                    scrollBehavior = scrollBehavior,
                )
            }
        },
        bottomBar = {
            // A picked selection puts its own toolbar along the bottom edge, and
            // two bars there would be a stack of targets with no clear owner.
            if (bottomBar != null && !screenOwnsChrome && !chrome.selectionActive) bottomBar()
        },
    ) { innerPadding ->
        // The capture surface sits over the screens rather than inside any one
        // of them: writing something down is reachable from everywhere, and the
        // sheet it raises has to survive moving between destinations.
        Box(modifier = Modifier.fillMaxSize().padding(innerPadding)) {
            AppNavHost(navController = navController, screens = screens)
            screens.quickAdd(!screenOwnsChrome && !screenOwnsCorner && !chrome.selectionActive)
        }
    }
}

/**
 * The navigation bar a phone gets under every list.
 *
 * It repeats four of the drawer's entries rather than replacing the drawer: the
 * places the day is spent between are a thumb away, and the rest of the catalog
 * keeps the home it has always had.
 */
@Composable
private fun BottomBar(
    selected: (DrawerDestination) -> Boolean,
    onSelect: (DrawerDestination) -> Unit,
) {
    NavigationBar(
        containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        modifier = Modifier.testTag(BOTTOM_BAR_TAG),
    ) {
        for (destination in BottomBarDestinations) {
            NavigationBarItem(
                selected = selected(destination),
                onClick = { onSelect(destination) },
                icon = { Icon(destination.icon, contentDescription = null) },
                label = { Text(stringResource(destination.labelRes), maxLines = 1) },
                colors =
                    NavigationBarItemDefaults.colors(
                        indicatorColor = MaterialTheme.colorScheme.primaryContainer,
                        selectedIconColor = MaterialTheme.colorScheme.onPrimaryContainer,
                    ),
            )
        }
    }
}

/** True when the drawer entry points at the destination currently on screen. */
private fun NavDestination?.isOn(destination: DrawerDestination): Boolean =
    this?.hasRoute(destination.route::class) == true

/** Title resource for whatever is on screen; the app name is the last resort. */
private fun titleResFor(destination: NavDestination?): Int =
    RouteTitles.all.entries
        .firstOrNull { (routeClass, _) -> destination?.hasRoute(routeClass) == true }
        ?.value
        ?: R.string.native_app_name
