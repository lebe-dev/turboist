package ru.tinyops.turboist.nativeapp.projects.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.PushPin
import androidx.compose.material.icons.outlined.ViewColumn
import androidx.compose.material3.AssistChip
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.projects.ProjectFilter
import ru.tinyops.turboist.nativeapp.projects.ProjectMessage
import ru.tinyops.turboist.nativeapp.projects.ProjectsUiState
import ru.tinyops.turboist.nativeapp.projects.ProjectsViewModel

/** What the projects screen can be asked to do. */
data class ProjectsCallbacks(
    val onNarrowTo: (ProjectFilter) -> Unit,
    val onSearch: (String) -> Unit,
    val onRefresh: () -> Unit,
    val onCreateProject: (contextLocalId: Long, title: String) -> Unit,
    val onOpenProject: (Project) -> Unit,
    val onOpenContext: (Context) -> Unit,
)

/**
 * The workspace, browsed.
 *
 * Projects are shown under the context they belong to rather than in one flat
 * list, because a context is where a project is filed and where the next one
 * will be: the heading is both the grouping and the place a project is added to.
 * Tapping a heading opens the context itself, which is the whole of the context
 * list this app needs — there is no second screen listing contexts on their own.
 *
 * The projects the user keeps to hand lead the screen, whichever context they
 * live in: that is what pinning is for.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProjectsScreen(
    state: ProjectsUiState,
    messages: Flow<ProjectMessage>,
    callbacks: ProjectsCallbacks,
    modifier: Modifier = Modifier,
) {
    val snackbars = remember { SnackbarHostState() }
    MessageHost(messages, snackbars)

    var addingTo by remember { mutableStateOf<Context?>(null) }

    Box(modifier = modifier.fillMaxSize()) {
        Column(modifier = Modifier.fillMaxSize()) {
            FilterRow(state, callbacks.onNarrowTo)
            OutlinedTextField(
                value = state.query,
                onValueChange = callbacks.onSearch,
                singleLine = true,
                placeholder = { Text(stringResource(R.string.page_projects_searchPlaceholder)) },
                modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
            )
            PullToRefreshBox(
                isRefreshing = state.refreshing,
                onRefresh = callbacks.onRefresh,
                modifier = Modifier.fillMaxSize(),
            ) {
                // The whole screen is replaced only when there is nothing to
                // file work under at all. A context whose projects the current
                // narrowing hides still shows its heading: the heading is where
                // the next project is added, and taking it away would leave the
                // user with a message and nowhere to act on it.
                if (state.groups.isEmpty() && state.pinned.isEmpty()) {
                    EmptyMessage(
                        title = stringResource(R.string.page_projects_emptyTitle),
                        description = stringResource(R.string.page_projects_emptyDescription),
                    )
                    return@PullToRefreshBox
                }
                LazyColumn(modifier = Modifier.fillMaxSize()) {
                    if (state.pinned.isNotEmpty()) {
                        item(key = "pinned-heading") { GroupHeading(stringResource(R.string.nav_pinned)) }
                        items(state.pinned, key = { "pinned-" + it.localId }) { project ->
                            ProjectRow(
                                project,
                                onOpen = { callbacks.onOpenProject(project) },
                                dailyPlanEnabled = state.dailyPlanEnabled,
                            )
                        }
                    }
                    for (group in state.groups) {
                        item(key = "context-" + group.context.localId) {
                            ContextHeading(
                                name = group.context.name,
                                onOpen = { callbacks.onOpenContext(group.context) },
                                onAdd = { addingTo = group.context },
                            )
                        }
                        items(group.projects, key = { "project-" + it.localId }) { project ->
                            ProjectRow(
                                project,
                                onOpen = { callbacks.onOpenProject(project) },
                                dailyPlanEnabled = state.dailyPlanEnabled,
                            )
                        }
                    }
                }
            }
        }
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }

    val target = addingTo
    if (target != null) {
        NameDialog(
            title = stringResource(R.string.dialog_project_newTitle),
            label = stringResource(R.string.common_title),
            initial = "",
            confirmLabel = stringResource(R.string.common_create),
            onConfirm = { title -> callbacks.onCreateProject(target.localId, title) },
            onDismiss = { addingTo = null },
        )
    }
}

/**
 * The same screen, driven by a view model.
 *
 * The stateless form above is the one that is exercised in tests; this is the
 * wiring the app uses, and it holds nothing of its own so the two cannot drift.
 */
@Composable
fun ProjectsScreen(
    onOpenProject: (Project) -> Unit,
    onOpenContext: (Context) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ProjectsViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    ProjectsScreen(
        state = state,
        messages = presenter.messages,
        callbacks =
            remember(presenter, onOpenProject, onOpenContext) {
                ProjectsCallbacks(
                    onNarrowTo = presenter::narrowTo,
                    onSearch = presenter::search,
                    onRefresh = presenter::refresh,
                    onCreateProject = presenter::createProject,
                    onOpenProject = onOpenProject,
                    onOpenContext = onOpenContext,
                )
            },
        modifier = modifier,
    )
}

@Composable
private fun FilterRow(
    state: ProjectsUiState,
    onNarrowTo: (ProjectFilter) -> Unit,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState())
                .padding(horizontal = 12.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        for (filter in ProjectFilter.entries) {
            FilterChip(
                selected = state.filter == filter,
                onClick = { onNarrowTo(filter) },
                label = {
                    Text(
                        stringResource(
                            R.string.native_count_suffix,
                            stringResource(filterLabel(filter)),
                            state.counts[filter] ?: 0,
                        ),
                    )
                },
            )
        }
    }
}

/** A context, as a heading over the projects filed under it. */
@Composable
private fun ContextHeading(
    name: String,
    onOpen: () -> Unit,
    onAdd: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onOpen).padding(start = 12.dp, top = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = name,
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurface,
            modifier = Modifier.weight(1f),
        )
        IconButton(onClick = onAdd) {
            Icon(
                imageVector = Icons.Filled.Add,
                contentDescription = stringResource(R.string.sidebar_addToAriaLabel, name),
            )
        }
    }
    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
}

@Composable
private fun GroupHeading(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, top = 12.dp, bottom = 4.dp),
    )
}

/**
 * One project as a row.
 *
 * Everything on it is a fact worth seeing without opening the project: the
 * colour it was given, whether it is hidden from the shared view, whether it
 * holds a place in the daily plan, what kind of work it is, and — when it is no
 * longer open — where it stands.
 *
 * [dailyPlanEnabled] is the user's preference for the plan. A workspace that
 * does not keep one is not told which projects stand in it: the mark would name
 * a place the user has no way to reach.
 */
@Composable
fun ProjectRow(
    project: Project,
    onOpen: () -> Unit,
    modifier: Modifier = Modifier,
    dailyPlanEnabled: Boolean = false,
) {
    val tint = projectTint(project.color)
    Row(
        modifier = modifier.fillMaxWidth().clickable(onClick = onOpen).padding(horizontal = 12.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Folder,
            contentDescription = null,
            tint = tint ?: MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(18.dp),
        )
        Text(
            text = project.title,
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        if (project.isPinned) {
            Icon(
                imageVector = Icons.Outlined.PushPin,
                contentDescription = stringResource(R.string.project_pinned),
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(14.dp),
            )
        }
        if (project.isPrivate) {
            Icon(
                imageVector = Icons.Outlined.Lock,
                contentDescription = stringResource(R.string.common_privateMarker),
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(14.dp),
            )
        }
        if (dailyPlanEnabled && project.troikiCategory != null) {
            Icon(
                imageVector = Icons.Outlined.ViewColumn,
                contentDescription = troikiLabel(project.troikiCategory!!)?.let { stringResource(it) },
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(14.dp),
            )
        }
        if (project.type == ProjectType.SOFTWARE) {
            AssistChip(onClick = onOpen, label = { Text(stringResource(R.string.dialog_project_typeSoftware)) })
        }
        val status = statusLabel(project.status)
        if (project.status != ProjectStatus.OPEN && status != null) {
            AssistChip(onClick = onOpen, label = { Text(stringResource(status)) })
        }
    }
}

/** What a screen says when it holds nothing, phrased as a state rather than a failure. */
@Composable
fun EmptyMessage(
    title: String,
    description: String,
    modifier: Modifier = Modifier,
) {
    Box(modifier = modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            modifier = Modifier.padding(32.dp),
        ) {
            Text(
                text = title,
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.onSurface,
                textAlign = TextAlign.Center,
            )
            Text(
                text = description,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
    }
}

/** Shows whatever a project screen says back, once each. */
@Composable
fun MessageHost(
    messages: Flow<ProjectMessage>,
    snackbars: SnackbarHostState,
) {
    var pending by remember { mutableStateOf<ProjectMessage?>(null) }
    LaunchedEffect(messages) { messages.collect { pending = it } }
    val message = pending ?: return
    val text = projectMessageText(message)
    LaunchedEffect(message, text) {
        snackbars.showSnackbar(text)
        pending = null
    }
}
