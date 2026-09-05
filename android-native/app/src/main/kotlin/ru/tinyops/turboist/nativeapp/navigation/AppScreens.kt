package ru.tinyops.turboist.nativeapp.navigation

import androidx.compose.runtime.Composable
import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.account.ui.ApiTokensScreen
import ru.tinyops.turboist.nativeapp.account.ui.SessionsScreen
import ru.tinyops.turboist.nativeapp.account.ui.TwoFactorScreen
import ru.tinyops.turboist.nativeapp.auth.passkey.ui.PasskeysScreen
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonEntry
import ru.tinyops.turboist.nativeapp.harpoon.ui.HarpoonAction
import ru.tinyops.turboist.nativeapp.labels.ui.LabelTasksScreen
import ru.tinyops.turboist.nativeapp.labels.ui.LabelsScreen
import ru.tinyops.turboist.nativeapp.projects.ui.ContextScreen
import ru.tinyops.turboist.nativeapp.projects.ui.ProjectScreen
import ru.tinyops.turboist.nativeapp.projects.ui.ProjectsScreen
import ru.tinyops.turboist.nativeapp.quickadd.ui.QuickAddHost
import ru.tinyops.turboist.nativeapp.search.SearchNavigation
import ru.tinyops.turboist.nativeapp.search.ui.SearchScreen
import ru.tinyops.turboist.nativeapp.settings.ui.SettingsDestinations
import ru.tinyops.turboist.nativeapp.settings.ui.SettingsScreen
import ru.tinyops.turboist.nativeapp.tasks.TaskAddress
import ru.tinyops.turboist.nativeapp.tasks.ui.CompletedScreen
import ru.tinyops.turboist.nativeapp.tasks.ui.InboxScreen
import ru.tinyops.turboist.nativeapp.tasks.ui.NextWeekScreen
import ru.tinyops.turboist.nativeapp.tasks.ui.TaskDetailScreen
import ru.tinyops.turboist.nativeapp.tasks.ui.TodayScreen
import ru.tinyops.turboist.nativeapp.tasks.ui.TomorrowScreen
import ru.tinyops.turboist.nativeapp.tasks.ui.WeekScreen
import ru.tinyops.turboist.nativeapp.templates.ui.TemplatesScreen
import ru.tinyops.turboist.nativeapp.troiki.ui.TroikiScreen
import ru.tinyops.turboist.nativeapp.unsent.ui.UnsentChangesScreen

/**
 * What fills each destination of the app graph.
 *
 * The graph names destinations; this says what appears in them. Holding the two
 * apart keeps a real screen and its dependencies out of the checks that are
 * about wiring — the drawer walk composes the whole shell, and a screen that
 * needs a database and a sync engine to appear would make every navigation
 * check drag both along.
 *
 * Each entry is handed the one thing the graph owns and a screen does not: how
 * to open a task from a row.
 */
class AppScreens(
    val today: @Composable ((Task) -> Unit) -> Unit = { open -> TodayScreen(onOpenTask = open) },
    val tomorrow: @Composable ((Task) -> Unit) -> Unit = { open -> TomorrowScreen(onOpenTask = open) },
    val week: @Composable ((Task) -> Unit) -> Unit = { open -> WeekScreen(onOpenTask = open) },
    val nextWeek: @Composable ((Task) -> Unit) -> Unit = { open -> NextWeekScreen(onOpenTask = open) },
    val inbox: @Composable ((Task) -> Unit) -> Unit = { open -> InboxScreen(onOpenTask = open) },
    /**
     * The completion history. It takes the same one thing every list does — how
     * to open a task from a row — even though part of what it shows was read from
     * the server rather than from the replica: a line with no device id yet opens
     * nothing, exactly as a row waiting to be sent does.
     */
    val completed: @Composable ((Task) -> Unit) -> Unit = { open -> CompletedScreen(onOpenTask = open) },
    /**
     * Takes the one thing the graph owns: where each settings row leads. A row
     * whose destination the screen navigated to itself would tie the settings
     * surface to the route names.
     */
    val settings: @Composable (SettingsDestinations) -> Unit = { destinations ->
        SettingsScreen(destinations = destinations)
    },
    /**
     * The reusable blueprints. Takes how to open a task, because using a template
     * makes work in a project the user is not looking at and the screen goes
     * there — a destination, and so the graph's.
     */
    val templates: @Composable ((Long) -> Unit) -> Unit = { openTask ->
        TemplatesScreen(onOpenTask = openTask)
    },
    /**
     * Takes how to open each of the four kinds a search can find. A result is a
     * record rather than an id, because only the graph knows a destination is
     * addressed by a server id and that a record made offline has none yet.
     */
    val search: @Composable (SearchNavigation) -> Unit = { navigation -> SearchScreen(navigation = navigation) },
    /**
     * One whole task, opened at whichever address brought the user here. Takes
     * how to open another task — a subtask, or the work that is blocking this one
     * — how to leave, and the jump-pair control. All three belong to the graph
     * rather than the screen: the first two are destinations, and the third is a
     * control that navigates to one. The screen draws its own top bar, which is
     * where that control has to sit.
     */
    val task: @Composable (TaskAddress, (Long) -> Unit, () -> Unit, @Composable () -> Unit) -> Unit =
        { address, open, back, harpoon ->
            TaskDetailScreen(address = address, onOpenTask = open, onBack = back, harpoon = harpoon)
        },
    /**
     * The workspace, browsed. Takes how to open a project and how to open the
     * context a heading names — both destinations, and so both the graph's.
     */
    val projects: @Composable ((Project) -> Unit, (Context) -> Unit) -> Unit = { openProject, openContext ->
        ProjectsScreen(onOpenProject = openProject, onOpenContext = openContext)
    },
    /**
     * One project and its board. Takes the project to show, how to open a task
     * from a row, and how to leave — the last of which the screen needs because
     * deleting a project takes the screen with it.
     */
    val project: @Composable (Long, (Task) -> Unit, () -> Unit) -> Unit = { projectLocalId, openTask, leave ->
        ProjectScreen(projectLocalId = projectLocalId, onOpenTask = openTask, onLeave = leave)
    },
    /**
     * How often each label is being reached for. Takes how to open a label — a
     * destination, and so the graph's — because every row on the report leads to
     * the work carrying that label.
     */
    val labels: @Composable ((Label) -> Unit) -> Unit = { openLabel ->
        LabelsScreen(onOpenLabel = openLabel)
    },
    /**
     * One label and the work carrying it. Takes the label to show, how to open a
     * task from a row, and how to leave — the last of which the screen needs
     * because deleting a label takes the screen with it.
     */
    val label: @Composable (Long, (Task) -> Unit, () -> Unit) -> Unit = { labelLocalId, openTask, leave ->
        LabelTasksScreen(labelLocalId = labelLocalId, onOpenTask = openTask, onLeave = leave)
    },
    /** One context: its projects, and the work in the whole branch under it. */
    val context: @Composable (Long, (Project) -> Unit, (Task) -> Unit, () -> Unit) -> Unit =
        { contextLocalId, openProject, openTask, leave ->
            ContextScreen(
                contextLocalId = contextLocalId,
                onOpenProject = openProject,
                onOpenTask = openTask,
                onLeave = leave,
            )
        },
    /**
     * The daily plan. Takes how to open a task from a row and how to open one of
     * the projects standing in it — both destinations, and so both the graph's.
     */
    val troiki: @Composable ((Task) -> Unit, (Project) -> Unit) -> Unit = { openTask, openProject ->
        TroikiScreen(onOpenTask = openTask, onOpenProject = openProject)
    },
    /** Takes nothing from the graph: a credential belongs to the account, not to a row. */
    val passkeys: @Composable () -> Unit = { PasskeysScreen() },
    /**
     * The three administrative surfaces, each taking nothing from the graph:
     * nothing on them is a place, and every action they offer is about the
     * account rather than about the work.
     */
    val sessions: @Composable () -> Unit = { SessionsScreen() },
    val apiTokens: @Composable () -> Unit = { ApiTokensScreen() },
    val twoFactor: @Composable () -> Unit = { TwoFactorScreen() },
    /**
     * What the server has not been told. Takes nothing from the graph: every row
     * on it is a change rather than a place, and the only actions it offers act
     * on the queue itself.
     */
    val unsentChanges: @Composable () -> Unit = { UnsentChangesScreen() },
    /**
     * The jump pair, in the top bar: the two things the user is hopping between,
     * and the way the thing on screen is hooked onto the pair or taken off it.
     * Takes what is on screen, when that is a task or a project, and how to open
     * an end of the pair — a destination, and so the graph's.
     */
    val harpoon: @Composable (HarpoonEntry?, (HarpoonEntry) -> Unit) -> Unit = { current, open ->
        HarpoonAction(current = current, onOpen = open)
    },
    /**
     * The capture surface — the button that opens it and the sheet it opens —
     * drawn over whichever screen is on top. It is an entry here rather than a
     * fixture of the shell so a check about navigation can compose the shell
     * without a database and a sync engine behind it.
     *
     * It is told whether to draw its button, and stays composed either way: a
     * share from another app opens the sheet without anyone touching the button,
     * so a screen that does not want the button — one that has a control of its
     * own in the same corner — must not take the sheet down with it.
     */
    val quickAdd: @Composable (Boolean) -> Unit = { showButton -> QuickAddHost(showButton = showButton) },
)
