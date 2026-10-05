package ru.tinyops.turboist.nativeapp.search.ui

import androidx.annotation.StringRes
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
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Label
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Workspaces
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.search.SearchKind
import ru.tinyops.turboist.nativeapp.search.SearchNavigation
import ru.tinyops.turboist.nativeapp.search.SearchPresenter
import ru.tinyops.turboist.nativeapp.search.SearchResults
import ru.tinyops.turboist.nativeapp.search.SearchUiState
import ru.tinyops.turboist.nativeapp.search.SearchViewModel
import ru.tinyops.turboist.nativeapp.search.TaskHit

/** What the search screen can be asked to do. Gathered so a screen takes one object, not ten lambdas. */
data class SearchCallbacks(
    val onType: (String) -> Unit,
    val onSubmit: () -> Unit,
    val onNarrowTo: (SearchKind?) -> Unit,
    val onToggleOpenOnly: () -> Unit,
    val onRerun: (String) -> Unit,
    val onForgetRecent: () -> Unit,
    val navigation: SearchNavigation,
)

/**
 * Search over everything the device holds.
 *
 * The screen has three states and they are not variations of one another: it is
 * resting until there is enough to search for, it is answering, or it is showing
 * an answer. What it never has is a network state — the index is local, so there
 * is nothing to wait for, nothing to retry and no reason for a search to behave
 * differently on a train than at a desk.
 *
 * Results stay grouped by kind rather than interleaved. Ranking within a kind is
 * a question of relevance and is answered in the query; ranking a label against
 * a task is not a question with an answer, and mixing them would only make both
 * harder to scan.
 */
@Composable
fun SearchScreen(
    state: SearchUiState,
    callbacks: SearchCallbacks,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxSize()) {
        QueryField(state, callbacks)
        KindFilters(state, callbacks)
        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
        Box(modifier = Modifier.fillMaxSize()) {
            when {
                state.resting -> RestingState(state.recent, callbacks)
                state.searching && !state.answered -> Notice(stringResource(R.string.page_search_searching))
                state.answered && state.results.isEmpty -> Notice(stringResource(R.string.native_search_noMatches))
                else -> ResultList(state.results, callbacks)
            }
        }
    }
}

/**
 * The same screen driven by a view model.
 *
 * The stateless form above is the one exercised in tests; this holds nothing of
 * its own, so the two cannot drift.
 */
@Composable
fun SearchScreen(
    navigation: SearchNavigation,
    modifier: Modifier = Modifier,
    viewModel: SearchViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()

    SearchScreen(
        state = state,
        callbacks = presenterCallbacks(presenter, navigation),
        modifier = modifier,
    )
}

@Composable
private fun presenterCallbacks(
    presenter: SearchPresenter,
    navigation: SearchNavigation,
): SearchCallbacks =
    remember(presenter, navigation) {
        SearchCallbacks(
            onType = presenter::type,
            onSubmit = presenter::submit,
            onNarrowTo = presenter::narrowTo,
            onToggleOpenOnly = presenter::toggleOpenTasksOnly,
            onRerun = presenter::rerun,
            onForgetRecent = presenter::forgetRecent,
            navigation = navigation,
        )
    }

@Composable
private fun QueryField(
    state: SearchUiState,
    callbacks: SearchCallbacks,
) {
    OutlinedTextField(
        value = state.typed,
        onValueChange = callbacks.onType,
        singleLine = true,
        placeholder = { Text(stringResource(R.string.page_search_placeholder)) },
        leadingIcon = { Icon(Icons.Outlined.Search, contentDescription = null) },
        trailingIcon = {
            if (state.typed.isNotEmpty()) {
                IconButton(onClick = { callbacks.onType("") }) {
                    Icon(
                        imageVector = Icons.Filled.Close,
                        contentDescription = stringResource(R.string.native_search_clearQuery),
                    )
                }
            }
        },
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
        keyboardActions = KeyboardActions(onSearch = { callbacks.onSubmit() }),
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

/**
 * Which kind of thing to look at, and whether finished work counts.
 *
 * The kinds are one choice with five settings, the last chip a separate one: a
 * chip row that mixed the two would suggest "Open only" excluded projects,
 * labels and contexts, which have no such state to be excluded by.
 */
@Composable
private fun KindFilters(
    state: SearchUiState,
    callbacks: SearchCallbacks,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        FilterChip(
            selected = state.filters.kind == null,
            onClick = { callbacks.onNarrowTo(null) },
            label = { Text(stringResource(R.string.page_projects_filterAll)) },
        )
        for (kind in SearchKind.entries) {
            FilterChip(
                selected = state.filters.kind == kind,
                onClick = { callbacks.onNarrowTo(kind) },
                label = { Text(stringResource(kindLabel(kind))) },
            )
        }
        FilterChip(
            selected = state.filters.openTasksOnly,
            onClick = { callbacks.onToggleOpenOnly() },
            label = { Text(stringResource(R.string.native_search_openOnly)) },
        )
    }
}

@StringRes
private fun kindLabel(kind: SearchKind): Int =
    when (kind) {
        SearchKind.TASKS -> R.string.page_search_tasksTab
        SearchKind.PROJECTS -> R.string.page_search_projectsTab
        SearchKind.LABELS -> R.string.nav_labels
        SearchKind.CONTEXTS -> R.string.native_search_contextsTab
    }

/**
 * What the screen shows before there is anything to search for.
 *
 * The past searches take the place of the explanation once there are any: a
 * person who has searched before does not need to be told what the field is for,
 * and their last few queries are the fastest way back to what they were doing.
 */
@Composable
private fun RestingState(
    recent: List<String>,
    callbacks: SearchCallbacks,
) {
    if (recent.isEmpty()) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Column(
                horizontalAlignment = Alignment.CenterHorizontally,
                modifier = Modifier.padding(32.dp),
            ) {
                Text(
                    text = stringResource(R.string.page_search_emptyTitle),
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onSurface,
                    textAlign = TextAlign.Center,
                )
                Text(
                    text = stringResource(R.string.native_search_emptyDescription),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
        }
        return
    }
    LazyColumn(modifier = Modifier.fillMaxSize()) {
        item(key = "recent-heading") {
            Row(
                modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 4.dp, top = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = stringResource(R.string.native_search_recent),
                    style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.weight(1f),
                )
                TextButton(onClick = callbacks.onForgetRecent) {
                    Text(stringResource(R.string.native_search_clearRecent))
                }
            }
        }
        items(recent, key = { "recent-$it" }) { query ->
            ResultRow(
                icon = Icons.Outlined.History,
                title = query,
                subtitle = null,
                onOpen = { callbacks.onRerun(query) },
            )
        }
    }
}

/** The results, one titled block per kind, empty blocks left out. */
@Composable
private fun ResultList(
    results: SearchResults,
    callbacks: SearchCallbacks,
) {
    LazyColumn(modifier = Modifier.fillMaxSize()) {
        kindBlock(SearchKind.TASKS, results.tasks.size) {
            items(results.tasks, key = { "task-" + it.task.localId }) { hit ->
                TaskResultRow(hit) { callbacks.navigation.onOpenTask(hit.task) }
            }
        }
        kindBlock(SearchKind.PROJECTS, results.projects.size) {
            items(results.projects, key = { "project-" + it.localId }) { project ->
                ResultRow(Icons.Outlined.Folder, project.title, null) {
                    callbacks.navigation.onOpenProject(project)
                }
            }
        }
        kindBlock(SearchKind.LABELS, results.labels.size) {
            items(results.labels, key = { "label-" + it.localId }) { label ->
                ResultRow(Icons.AutoMirrored.Outlined.Label, label.name, null) {
                    callbacks.navigation.onOpenLabel(label)
                }
            }
        }
        kindBlock(SearchKind.CONTEXTS, results.contexts.size) {
            items(results.contexts, key = { "context-" + it.localId }) { context ->
                ResultRow(Icons.Outlined.Workspaces, context.name, null) {
                    callbacks.navigation.onOpenContext(context)
                }
            }
        }
    }
}

/** A kind's heading and its rows, or nothing at all when the kind matched nothing. */
private fun LazyListScope.kindBlock(
    kind: SearchKind,
    count: Int,
    rows: LazyListScope.() -> Unit,
) {
    if (count == 0) return
    item(key = "heading-" + kind.name) { KindHeading(kind, count) }
    rows()
}

@Composable
private fun KindHeading(
    kind: SearchKind,
    count: Int,
) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = stringResource(kindLabel(kind)),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurface,
        )
        Text(
            text = count.toString(),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * A matching task.
 *
 * Finished work is struck through and says so, because search reaches into it on
 * purpose: without the marking, a completed task would read as something still
 * waiting to be done.
 */
@Composable
private fun TaskResultRow(
    hit: TaskHit,
    onOpen: () -> Unit,
) {
    val finished = hit.task.status.isClosed
    val titleColour =
        if (finished) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onOpen).padding(horizontal = 16.dp, vertical = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = if (hit.task.projectLocalId != null) Icons.Outlined.Folder else Icons.Outlined.Inbox,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.outline,
            modifier = Modifier.size(20.dp),
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = hit.task.title,
                style = MaterialTheme.typography.bodyLarge,
                color = titleColour,
                textDecoration = if (finished) TextDecoration.LineThrough else null,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            if (hit.projectTitle != null) {
                Text(
                    text = hit.projectTitle,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
        if (hit.task.status == TaskStatus.COMPLETED) {
            Icon(
                imageVector = Icons.Outlined.CheckCircle,
                contentDescription = stringResource(R.string.native_search_taskFinished),
                tint = MaterialTheme.colorScheme.outline,
                modifier = Modifier.size(20.dp),
            )
        }
    }
}

/** A result that is a name and nothing else: a project, a label, a context, a past query. */
@Composable
private fun ResultRow(
    icon: ImageVector,
    title: String,
    subtitle: String?,
    onOpen: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onOpen).padding(horizontal = 16.dp, vertical = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.outline,
            modifier = Modifier.size(20.dp),
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = title,
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (subtitle != null) {
                Text(
                    text = subtitle,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/** One line of plain text where the results would be: searching, or nothing found. */
@Composable
private fun Notice(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 24.dp),
        textAlign = TextAlign.Center,
    )
}
