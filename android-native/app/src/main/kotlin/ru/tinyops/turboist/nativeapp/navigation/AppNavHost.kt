package ru.tinyops.turboist.nativeapp.navigation

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.navDeepLink
import androidx.navigation.toRoute
import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.sync.write.HarpoonTarget
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonEntry
import ru.tinyops.turboist.nativeapp.search.SearchNavigation
import ru.tinyops.turboist.nativeapp.settings.ui.SettingsDestinations
import ru.tinyops.turboist.nativeapp.tasks.TaskAddress

/**
 * The graph shown to a signed-in user.
 *
 * Every destination is registered here even when its screen is still a stub, so
 * navigation wiring is complete and testable before the screens exist and a new
 * screen is a one-line swap rather than a graph change.
 */
@Composable
fun AppNavHost(
    navController: NavHostController,
    modifier: Modifier = Modifier,
    screens: AppScreens = AppScreens(),
) {
    // A row opens the task it names by the id this device holds it under, so a
    // task written down while offline opens like any other.
    val openTaskById: (Long) -> Unit = { localId -> navController.navigate(TaskRoute(localId)) }
    val openTask: (Task) -> Unit = { task -> openTaskById(task.localId) }

    // Leaving a detail screen goes back where the user came from. A link opened
    // from outside has nowhere behind it, so it falls through to the day view
    // rather than closing the app.
    val leave: () -> Unit = { if (!navController.popBackStack()) navController.navigate(TodayRoute) }

    // Jumping is a navigation like any other: the pair holds the ids this device
    // holds its rows under, so an end of it made offline leads somewhere too.
    val jump: (HarpoonEntry) -> Unit = { entry ->
        when (entry.target) {
            HarpoonTarget.TASK -> navController.navigate(TaskRoute(entry.localId))
            HarpoonTarget.PROJECT -> navController.navigate(ProjectRoute(entry.localId))
        }
    }

    // A project, a context and a label are opened by the id this device holds them
    // under, so one written down while offline leads somewhere like any other.
    val openProject: (Project) -> Unit = { project -> navController.navigate(ProjectRoute(project.localId)) }
    val openContext: (Context) -> Unit = { context -> navController.navigate(ContextRoute(context.localId)) }
    val openLabel: (Label) -> Unit = { label -> navController.navigate(LabelRoute(label.localId)) }

    NavHost(
        navController = navController,
        startDestination = TodayRoute,
        modifier = modifier,
    ) {
        composable<TodayRoute> { screens.today(openTask) }
        composable<TomorrowRoute> { screens.tomorrow(openTask) }
        composable<WeekRoute> { screens.week(openTask) }
        composable<NextWeekRoute> { screens.nextWeek(openTask) }
        composable<InboxRoute> { screens.inbox(openTask) }
        composable<CompletedRoute> { screens.completed(openTask) }
        composable<TroikiRoute> { screens.troiki(openTask, openProject) }
        composable<SearchRoute> {
            screens.search(
                SearchNavigation(
                    onOpenTask = openTask,
                    // Every result is opened by the id this device holds it
                    // under, so one written down offline leads somewhere like any
                    // other.
                    onOpenProject = { project -> navController.navigate(ProjectRoute(project.localId)) },
                    onOpenLabel = { label -> openLabel(label) },
                    onOpenContext = { context -> navController.navigate(ContextRoute(context.localId)) },
                ),
            )
        }
        composable<SettingsRoute> {
            screens.settings(
                SettingsDestinations(
                    openPasskeys = { navController.navigate(PasskeysRoute) },
                    openSessions = { navController.navigate(SessionsRoute) },
                    openApiTokens = { navController.navigate(ApiTokensRoute) },
                    openTwoFactor = { navController.navigate(TwoFactorRoute) },
                    openTemplates = { navController.navigate(TemplatesRoute) },
                    openUnsentChanges = { navController.navigate(UnsentChangesRoute) },
                ),
            )
        }
        composable<PasskeysRoute> { screens.passkeys() }
        composable<SessionsRoute> { screens.sessions() }
        composable<ApiTokensRoute> { screens.apiTokens() }
        composable<TwoFactorRoute> { screens.twoFactor() }
        composable<UnsentChangesRoute> { screens.unsentChanges() }
        composable<TemplatesRoute> { screens.templates(openTaskById) }

        composable<ProjectsRoute> { screens.projects(openProject, openContext) }
        composable<ProjectRoute> { entry ->
            screens.project(entry.toRoute<ProjectRoute>().projectLocalId, openTask, leave)
        }
        composable<LabelsRoute> { screens.labels(openLabel) }
        composable<LabelRoute> { entry ->
            screens.label(entry.toRoute<LabelRoute>().labelLocalId, openTask, leave)
        }
        composable<ContextRoute> { entry ->
            screens.context(entry.toRoute<ContextRoute>().contextLocalId, openProject, openTask, leave)
        }

        composable<TaskRoute> { entry ->
            val taskLocalId = entry.toRoute<TaskRoute>().taskLocalId
            screens.task(TaskAddress.Local(taskLocalId), openTaskById, leave) {
                screens.harpoon(HarpoonEntry(HarpoonTarget.TASK, taskLocalId), jump)
            }
        }

        // The one destination a link from outside can land on: opening
        // `turboist://task/<server id>` lands on the same task the web client
        // would show for `/task/<server id>`. It shows the same screen, which
        // resolves the server's id against the replica.
        composable<TaskLinkRoute>(
            deepLinks = listOf(navDeepLink<TaskLinkRoute>(basePath = DeepLinks.TASK_BASE_PATH)),
        ) { entry ->
            // A link from outside carries the server's id, and the pair is
            // remembered by the ids this device holds its rows under. There is
            // nothing to hook on until the task has been opened normally, so the
            // control is handed no current entry rather than a wrong one.
            screens.task(TaskAddress.Server(entry.toRoute<TaskLinkRoute>().serverId), openTaskById, leave) {
                screens.harpoon(null, jump)
            }
        }
    }
}
