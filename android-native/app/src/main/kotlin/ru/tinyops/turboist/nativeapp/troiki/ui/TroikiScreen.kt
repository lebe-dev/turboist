package ru.tinyops.turboist.nativeapp.troiki.ui

import androidx.annotation.StringRes
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.projects.ProjectMessage
import ru.tinyops.turboist.nativeapp.projects.ui.MessageHost
import ru.tinyops.turboist.nativeapp.projects.ui.NameDialog
import ru.tinyops.turboist.nativeapp.projects.ui.troikiLabel
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import ru.tinyops.turboist.nativeapp.tasks.ui.TaskRow
import ru.tinyops.turboist.nativeapp.troiki.TroikiProjectCard
import ru.tinyops.turboist.nativeapp.troiki.TroikiSlot
import ru.tinyops.turboist.nativeapp.troiki.TroikiUiState
import ru.tinyops.turboist.nativeapp.troiki.TroikiViewModel
import java.time.LocalDate
import java.time.ZoneId

/** What the daily plan can be asked to do. */
data class TroikiCallbacks(
    val onRefresh: () -> Unit,
    val onOpenTask: (Task) -> Unit,
    val onOpenProject: (Project) -> Unit,
    val onToggleComplete: (Task) -> Unit,
    val onAddTask: (projectLocalId: Long, title: String) -> Unit,
    val onAssign: (projectLocalId: Long, category: TroikiCategory) -> Unit,
    val onRemove: (projectLocalId: Long) -> Unit,
    val onStart: () -> Unit,
    val onReset: () -> Unit,
)

/**
 * The daily plan: three buckets, the projects standing in them, and their work.
 *
 * Drawn as one column rather than three side by side, because a phone is a tall
 * screen and a bucket read sideways would put the other two out of sight.
 *
 * Both controls of the cycle are offered at all times. Whether a cycle is
 * currently running is a counter the server keeps beside the account and not a
 * record this device holds a copy of, so the screen cannot know which of the two
 * to hide — and it does not need to: beginning a running cycle changes nothing,
 * and so does ending a stopped one.
 *
 * Each bucket shows the room it *starts* with. A bucket earns more by work
 * finished in the bucket above it, and that number is the server's alone, so
 * what is drawn here is the floor: the places that are certainly still free.
 * Filling one the server has since taken is answered by the server, and the
 * plainly hopeless attempt is answered here, before the user leaves the screen.
 */
@Composable
fun TroikiScreen(
    state: TroikiUiState,
    zone: ZoneId,
    today: LocalDate,
    messages: Flow<ProjectMessage>,
    callbacks: TroikiCallbacks,
    modifier: Modifier = Modifier,
) {
    val snackbars = remember { SnackbarHostState() }
    MessageHost(messages, snackbars)

    var rulesShown by rememberSaveable { mutableStateOf(false) }
    var resetting by remember { mutableStateOf(false) }
    var addingTo by remember { mutableStateOf<TroikiProjectCard?>(null) }
    var filling by remember { mutableStateOf<TroikiCategory?>(null) }
    var collapsed by rememberSaveable { mutableStateOf(setOf<Long>()) }

    Box(modifier = modifier.fillMaxSize()) {
        Column(modifier = Modifier.fillMaxSize()) {
            CycleControls(
                rulesShown = rulesShown,
                onToggleRules = { rulesShown = !rulesShown },
                onStart = callbacks.onStart,
                onReset = { resetting = true },
            )
            if (rulesShown) RulesCard()
            PullToRefreshBox(
                isRefreshing = state.refreshing,
                onRefresh = callbacks.onRefresh,
                modifier = Modifier.fillMaxSize(),
            ) {
                LazyColumn(modifier = Modifier.fillMaxSize()) {
                    for (slot in state.slots) {
                        item(key = "heading-" + slot.category.wire) { SlotHeading(slot) }
                        for (card in slot.projects) {
                            projectCard(
                                card = card,
                                zone = zone,
                                today = today,
                                collapsed = card.project.localId in collapsed,
                                onToggleCollapsed = {
                                    collapsed =
                                        if (card.project.localId in collapsed) {
                                            collapsed - card.project.localId
                                        } else {
                                            collapsed + card.project.localId
                                        }
                                },
                                onAdd = { addingTo = card },
                                callbacks = callbacks,
                            )
                        }
                        items(
                            count = slot.freeSlots,
                            key = { index -> "free-" + slot.category.wire + "-" + index },
                        ) {
                            EmptySlot(onClick = { filling = slot.category })
                        }
                    }
                }
            }
        }
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }

    if (resetting) {
        ConfirmResetDialog(
            onConfirm = callbacks.onReset,
            onDismiss = { resetting = false },
        )
    }
    addingTo?.let { card ->
        NameDialog(
            title = stringResource(R.string.troiki_addTask),
            label = stringResource(R.string.common_title),
            initial = "",
            confirmLabel = stringResource(R.string.common_add),
            onConfirm = { title -> callbacks.onAddTask(card.project.localId, title) },
            onDismiss = { addingTo = null },
        )
    }
    filling?.let { category ->
        AssignSheet(
            category = category,
            candidates = state.assignable,
            onPick = { project -> callbacks.onAssign(project.localId, category) },
            onDismiss = { filling = null },
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
fun TroikiScreen(
    onOpenTask: (Task) -> Unit,
    onOpenProject: (Project) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: TroikiViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    val today by viewModel.today.collectAsStateWithLifecycle()

    TroikiScreen(
        state = state,
        zone = viewModel.zone,
        today = today,
        messages = presenter.messages,
        callbacks =
            remember(presenter, onOpenTask, onOpenProject) {
                TroikiCallbacks(
                    onRefresh = presenter::refresh,
                    onOpenTask = onOpenTask,
                    onOpenProject = onOpenProject,
                    onToggleComplete = presenter::toggleComplete,
                    onAddTask = presenter::addTask,
                    onAssign = presenter::assign,
                    onRemove = presenter::remove,
                    onStart = presenter::start,
                    onReset = presenter::reset,
                )
            },
        modifier = modifier,
    )
}

/** Beginning and ending a cycle, and the rules of the method beside them. */
@Composable
private fun CycleControls(
    rulesShown: Boolean,
    onToggleRules: () -> Unit,
    onStart: () -> Unit,
    onReset: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Button(onClick = onStart) { Text(stringResource(R.string.troiki_start)) }
        OutlinedButton(onClick = onReset) { Text(stringResource(R.string.troiki_reset)) }
        Box(modifier = Modifier.weight(1f))
        IconButton(onClick = onToggleRules) {
            Icon(
                imageVector = Icons.Outlined.Info,
                contentDescription = stringResource(R.string.troiki_howAria),
                tint =
                    if (rulesShown) {
                        MaterialTheme.colorScheme.primary
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    },
            )
        }
    }
}

/** The method, in the product's own words. */
@Composable
private fun RulesCard() {
    Column(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Text(stringResource(R.string.troiki_rulesTitle), style = MaterialTheme.typography.titleSmall)
        Note(stringResource(R.string.troiki_rulesIntro))
        Note(
            stringResource(R.string.troiki_section_important) + " — " +
                stringResource(R.string.troiki_rules_importantHint),
        )
        Note(
            stringResource(R.string.troiki_section_medium) + " — " +
                stringResource(R.string.troiki_rules_mediumHint),
        )
        Note(
            stringResource(R.string.troiki_section_rest) + " — " +
                stringResource(R.string.troiki_rules_restHint),
        )
        Note(stringResource(R.string.troiki_rules_bullet1))
        Note(stringResource(R.string.troiki_rules_bullet2))
        Note(stringResource(R.string.troiki_rules_bullet3))
        Note(stringResource(R.string.troiki_rules_bullet4))
        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
    }
}

/** A bucket's name, how full it is, and what it is for. */
@Composable
private fun SlotHeading(slot: TroikiSlot) {
    Column(
        modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 16.dp, bottom = 4.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            val name = troikiLabel(slot.category)?.let { stringResource(it) }
            if (name != null) Text(name, style = MaterialTheme.typography.titleMedium)
            Text(
                text =
                    stringResource(
                        R.string.page_weekSummary_troikiSlots,
                        slot.projects.size.toString(),
                        slot.capacity.toString(),
                    ),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        slotDescription(slot.category)?.let { Note(stringResource(it)) }
    }
}

/**
 * One project of the plan, with its work under it.
 *
 * Written as list items rather than as one composable, so a project holding a
 * hundred tasks costs the list what a hundred rows cost and not what a hundred
 * rows inside one item would.
 */
private fun LazyListScope.projectCard(
    card: TroikiProjectCard,
    zone: ZoneId,
    today: LocalDate,
    collapsed: Boolean,
    onToggleCollapsed: () -> Unit,
    onAdd: () -> Unit,
    callbacks: TroikiCallbacks,
) {
    val project = card.project
    item(key = "project-" + project.localId) {
        ProjectHeading(
            card = card,
            collapsed = collapsed,
            onToggleCollapsed = onToggleCollapsed,
            onAdd = onAdd,
            onOpen = { callbacks.onOpenProject(project) },
            onRemove = { callbacks.onRemove(project.localId) },
        )
    }
    if (collapsed) return
    if (card.isEmpty) {
        item(key = "no-tasks-" + project.localId) { Note(stringResource(R.string.troiki_noTasks), inset = true) }
    }
    items(card.open, key = { "task-" + it.task.localId }) { row ->
        PlanTaskRow(row, zone, today, callbacks)
    }
    if (card.done.isNotEmpty()) {
        item(key = "done-" + project.localId) {
            Note(stringResource(R.string.native_project_finished), inset = true)
        }
        items(card.done, key = { "done-task-" + it.task.localId }) { row ->
            PlanTaskRow(row, zone, today, callbacks)
        }
    }
}

@Composable
private fun ProjectHeading(
    card: TroikiProjectCard,
    collapsed: Boolean,
    onToggleCollapsed: () -> Unit,
    onAdd: () -> Unit,
    onOpen: () -> Unit,
    onRemove: () -> Unit,
) {
    val project = card.project
    var menu by remember { mutableStateOf(false) }
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 8.dp, end = 4.dp, top = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        IconButton(onClick = onToggleCollapsed) {
            Icon(
                imageVector = if (collapsed) Icons.Filled.ExpandMore else Icons.Filled.ExpandLess,
                contentDescription =
                    stringResource(
                        if (collapsed) R.string.troiki_expandProject else R.string.troiki_collapseProject,
                        project.title,
                    ),
            )
        }
        Text(
            text = project.title,
            style = MaterialTheme.typography.titleSmall,
            modifier = Modifier.clickable(onClick = onOpen),
        )
        Text(
            text = card.open.size.toString(),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        IconButton(onClick = onAdd) {
            Icon(
                imageVector = Icons.Filled.Add,
                contentDescription = stringResource(R.string.troiki_addTaskAria, project.title),
            )
        }
        Box {
            IconButton(onClick = { menu = true }) {
                Icon(
                    imageVector = Icons.Filled.MoreVert,
                    contentDescription = stringResource(R.string.project_actionsAriaLabel),
                )
            }
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.project_removeFromTroiki)) },
                    onClick = {
                        menu = false
                        onRemove()
                    },
                )
            }
        }
    }
}

@Composable
private fun PlanTaskRow(
    row: TaskListRow,
    zone: ZoneId,
    today: LocalDate,
    callbacks: TroikiCallbacks,
) {
    TaskRow(
        row = row,
        zone = zone,
        today = today,
        selectionMode = false,
        selected = false,
        onToggleComplete = { callbacks.onToggleComplete(row.task) },
        onOpen = { callbacks.onOpenTask(row.task) },
        onSelectToggle = {},
        onStartSelection = {},
    )
}

/** A place in a bucket nothing stands in yet. Tapping it is how a project takes it. */
@Composable
private fun EmptySlot(onClick: () -> Unit) {
    Text(
        text = stringResource(R.string.troiki_emptySlot),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier =
            Modifier
                .fillMaxWidth()
                .clickable(onClick = onClick)
                .padding(horizontal = 16.dp, vertical = 12.dp),
    )
}

/** The projects a bucket can be filled with: open work that stands in no bucket yet. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AssignSheet(
    category: TroikiCategory,
    candidates: List<Project>,
    onPick: (Project) -> Unit,
    onDismiss: () -> Unit,
) {
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState()) {
        val name = troikiLabel(category)?.let { stringResource(it) }
        Text(
            text =
                stringResource(R.string.project_assignToTroiki) +
                    if (name != null) ": $name" else "",
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        if (candidates.isEmpty()) {
            Note(stringResource(R.string.page_projects_emptyTitle), inset = true)
        }
        // Scrollable rather than a plain stack: a workspace can hold far more
        // projects than a sheet is tall, and the ones past the fold would
        // otherwise be unreachable.
        Column(modifier = Modifier.verticalScroll(rememberScrollState())) {
            for (project in candidates) {
                ListItem(
                    headlineContent = { Text(project.title) },
                    modifier =
                        Modifier.clickable {
                            onPick(project)
                            onDismiss()
                        },
                )
            }
        }
    }
}

/** Ending a cycle takes every project out of the plan, so it is asked about first. */
@Composable
private fun ConfirmResetDialog(
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.troiki_resetTitle)) },
        text = { Text(stringResource(R.string.troiki_resetDescription)) },
        confirmButton = {
            TextButton(
                onClick = {
                    onConfirm()
                    onDismiss()
                },
            ) { Text(stringResource(R.string.troiki_resetConfirm)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

@Composable
private fun Note(
    text: String,
    inset: Boolean = false,
) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier =
            Modifier
                .fillMaxWidth()
                .padding(start = if (inset) 24.dp else 0.dp, top = 2.dp, bottom = 2.dp, end = 16.dp),
    )
}

/** What a bucket is for, in the product's own words. */
@StringRes
private fun slotDescription(category: TroikiCategory): Int? =
    when (category) {
        TroikiCategory.IMPORTANT -> R.string.troiki_section_importantDescription
        TroikiCategory.MEDIUM -> R.string.troiki_section_mediumDescription
        TroikiCategory.REST -> R.string.troiki_section_restDescription
        TroikiCategory.UNKNOWN -> null
    }
