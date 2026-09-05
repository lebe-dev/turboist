package ru.tinyops.turboist.nativeapp.quickadd.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.outlined.AutoAwesome
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.InputChip
import androidx.compose.material3.InputChipDefaults
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddMessage
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddPresenter
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddUiState
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddViewModel
import ru.tinyops.turboist.nativeapp.tasks.ui.DayPartSelector
import ru.tinyops.turboist.nativeapp.tasks.ui.LabelSelector
import ru.tinyops.turboist.nativeapp.tasks.ui.PrioritySelector
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/** Every gesture the capture sheet can make. */
data class QuickAddCallbacks(
    val onDismiss: () -> Unit = {},
    val onTitles: (String) -> Unit = {},
    val onDescription: (String) -> Unit = {},
    val onPriority: (Priority) -> Unit = {},
    val onDayPart: (DayPart) -> Unit = {},
    val onDueDate: (LocalDate?) -> Unit = {},
    val onLabels: (List<String>) -> Unit = {},
    val onRejectAutoLabel: (String) -> Unit = {},
    val onChooseProject: (Long?) -> Unit = {},
    val onPickingProject: (Boolean) -> Unit = {},
    val onProjectQuery: (String) -> Unit = {},
    val onSubmit: () -> Unit = {},
)

/**
 * The capture surface, drawn over whichever screen is on top.
 *
 * Writing something down is what this app is most often opened to do, so the way
 * in is on every screen and always in the same corner. The sheet it opens is the
 * only place in the app that creates a task from nothing — whether it was opened
 * by that button, by the launcher's own shortcut, or by another app sharing text.
 *
 * All of it works with no network: the task is in the replica and on the lists
 * the moment the sheet closes, and the queue reaches the server whenever the
 * phone next can.
 */
@Composable
fun QuickAddHost(
    modifier: Modifier = Modifier,
    viewModel: QuickAddViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    val today by viewModel.today.collectAsStateWithLifecycle()

    QuickAddHost(
        state = state,
        today = today,
        messages = presenter.messages,
        callbacks = quickAddCallbacks(presenter),
        onOpen = { presenter.open() },
        modifier = modifier,
    )
}

/**
 * The same surface without the dependency graph behind it.
 *
 * This is the form the checks drive; the one above is the wiring the app uses and
 * holds nothing of its own, so the two cannot drift apart.
 */
@Composable
fun QuickAddHost(
    state: QuickAddUiState,
    today: LocalDate,
    messages: Flow<QuickAddMessage>,
    callbacks: QuickAddCallbacks,
    onOpen: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val snackbars = remember { SnackbarHostState() }
    val addedToInbox = stringResource(R.string.task_toast_addedToInbox)
    val addedToProject = stringResource(R.string.task_toast_addedToProject)
    val failed = stringResource(R.string.task_toast_failedAdd)

    LaunchedEffect(messages, addedToInbox, addedToProject, failed) {
        messages.collect { message ->
            snackbars.showSnackbar(
                when (message) {
                    QuickAddMessage.ADDED_TO_INBOX -> addedToInbox
                    QuickAddMessage.ADDED_TO_PROJECT -> addedToProject
                    QuickAddMessage.FAILED -> failed
                },
            )
        }
    }

    Box(modifier = modifier.fillMaxSize()) {
        FloatingActionButton(
            onClick = onOpen,
            modifier = Modifier.align(Alignment.BottomEnd).padding(16.dp),
        ) {
            Icon(
                imageVector = Icons.Filled.Add,
                contentDescription = stringResource(R.string.task_addTask),
            )
        }
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }

    if (state.visible) {
        QuickAddSheet(state = state, today = today, callbacks = callbacks)
    }
}

/** Wires the sheet's gestures onto the object behind it. */
@Composable
fun quickAddCallbacks(presenter: QuickAddPresenter): QuickAddCallbacks =
    remember(presenter) {
        QuickAddCallbacks(
            onDismiss = presenter::dismiss,
            onTitles = presenter::setTitles,
            onDescription = presenter::setDescription,
            onPriority = presenter::setPriority,
            onDayPart = presenter::setDayPart,
            onDueDate = presenter::setDueDate,
            onLabels = presenter::setLabels,
            onRejectAutoLabel = presenter::rejectAutoLabel,
            onChooseProject = presenter::chooseProject,
            onPickingProject = presenter::setPickingProject,
            onProjectQuery = presenter::setProjectQuery,
            onSubmit = presenter::submit,
        )
    }

/** The sheet the surface raises, holding [QuickAddSheetContent]. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun QuickAddSheet(
    state: QuickAddUiState,
    today: LocalDate,
    callbacks: QuickAddCallbacks,
    modifier: Modifier = Modifier,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(
        onDismissRequest = callbacks.onDismiss,
        sheetState = sheetState,
        modifier = modifier,
    ) {
        QuickAddSheetContent(state = state, today = today, callbacks = callbacks)
    }
}

/**
 * What the sheet holds: either the task being written, or the list of places it
 * could go.
 *
 * The two replace each other rather than stacking. A picker raised on top of a
 * sheet on a phone leaves nothing of either visible, and the choice being made is
 * about the task on screen — so it takes the screen.
 */
@Composable
fun QuickAddSheetContent(
    state: QuickAddUiState,
    today: LocalDate,
    callbacks: QuickAddCallbacks,
    modifier: Modifier = Modifier,
) {
    if (state.pickingProject) {
        ProjectPicker(state = state, callbacks = callbacks, modifier = modifier)
    } else {
        QuickAddFields(state = state, today = today, callbacks = callbacks, modifier = modifier)
    }
}

@Composable
private fun QuickAddFields(
    state: QuickAddUiState,
    today: LocalDate,
    callbacks: QuickAddCallbacks,
    modifier: Modifier = Modifier,
) {
    val focus = remember { FocusRequester() }
    // The sheet exists to be typed into, so it opens with the cursor already in
    // the title. Anything else costs a tap on the most-used surface in the app.
    LaunchedEffect(Unit) { focus.requestFocus() }
    val titleLabel = stringResource(R.string.dialog_quickAdd_titleAriaLabel)
    val descriptionLabel = stringResource(R.string.dialog_quickAdd_descriptionAriaLabel)

    Column(
        modifier = modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(bottom = 24.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Text(
            text = stringResource(R.string.dialog_quickAdd_title),
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )

        OutlinedTextField(
            value = state.draft.titles,
            onValueChange = callbacks.onTitles,
            placeholder = { Text(stringResource(R.string.dialog_quickAdd_titlePlaceholder)) },
            modifier =
                Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp)
                    .focusRequester(focus)
                    .semantics { contentDescription = titleLabel },
        )

        OutlinedTextField(
            value = state.draft.description,
            onValueChange = callbacks.onDescription,
            placeholder = { Text(stringResource(R.string.dialog_quickAdd_descriptionPlaceholder)) },
            modifier =
                Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp)
                    .semantics { contentDescription = descriptionLabel },
        )

        AutoLabelChips(names = state.autoLabels, onReject = callbacks.onRejectAutoLabel)
        SuggestedProjectChips(projects = state.suggestedProjects, onChoose = callbacks.onChooseProject)

        DestinationRow(projectTitle = state.projectTitle, onPick = { callbacks.onPickingProject(true) })

        // A date is a plan, and the inbox holds what has not been planned yet, so
        // scheduling is offered only once the task has a home. The same rule
        // hides these controls in the web client.
        if (!state.draft.isInbox) {
            DueDateRow(dueDate = state.draft.dueDate, today = today, onPick = callbacks.onDueDate)
        }

        PrioritySelector(priority = state.draft.priority, onSelect = callbacks.onPriority)
        DayPartSelector(dayPart = state.draft.dayPart, onSelect = callbacks.onDayPart)
        LabelSelector(
            known = state.knownLabels,
            selected = state.draft.labelNames,
            onChange = callbacks.onLabels,
        )

        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
        ) {
            TextButton(onClick = callbacks.onDismiss) { Text(stringResource(R.string.common_cancel)) }
            Button(
                onClick = callbacks.onSubmit,
                enabled = state.draft.canSubmit && !state.submitting,
            ) {
                Text(submitLabel(state.draft.titleLines.size))
            }
        }
    }
}

/** The wording on the save button: one task, or how many. */
@Composable
private fun submitLabel(lines: Int): String =
    if (lines > 1) {
        stringResource(R.string.dialog_quickAdd_submitMulti, lines)
    } else {
        stringResource(R.string.dialog_quickAdd_submitSingle)
    }

/**
 * The labels the title has earned, each with a way to take it off.
 *
 * They are shown before the task is saved rather than after, because that is the
 * only moment the user can still say no: the rules are applied on save, on this
 * device and again on the server, and the refusal has to travel with the write
 * for the server to honour it.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun AutoLabelChips(
    names: List<String>,
    onReject: (String) -> Unit,
) {
    if (names.isEmpty()) return
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
        Text(
            text = stringResource(R.string.dialog_quickAdd_autoLabelHint),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            for (name in names) {
                val reject = stringResource(R.string.dialog_quickAdd_rejectAutoLabel, name)
                // Drawn in the accent rather than in the neutral a picked chip
                // would take, as the web client draws them: these are the one
                // thing on the sheet the user did not type, and they read as
                // something to check rather than as something already agreed.
                InputChip(
                    selected = true,
                    onClick = { onReject(name) },
                    label = { Text(name) },
                    leadingIcon = { Icon(Icons.Outlined.AutoAwesome, contentDescription = null) },
                    trailingIcon = { Icon(Icons.Outlined.Close, contentDescription = null) },
                    colors =
                        InputChipDefaults.inputChipColors(
                            selectedContainerColor =
                                MaterialTheme.colorScheme.primary.copy(alpha = AUTO_LABEL_TINT_ALPHA),
                            selectedLabelColor = MaterialTheme.colorScheme.primary,
                            selectedLeadingIconColor = MaterialTheme.colorScheme.primary,
                            selectedTrailingIconColor = MaterialTheme.colorScheme.primary,
                        ),
                    border =
                        BorderStroke(
                            width = 1.dp,
                            color = MaterialTheme.colorScheme.primary.copy(alpha = AUTO_LABEL_BORDER_ALPHA),
                        ),
                    modifier = Modifier.semantics { contentDescription = reject },
                )
            }
        }
    }
}

/**
 * The projects the title brings to mind.
 *
 * Advisory only — nothing moves until one is tapped. That is the whole difference
 * between these and the label chips above them, and it is deliberate: where work
 * belongs is a decision the user makes, not one a title mask makes for them.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun SuggestedProjectChips(
    projects: List<Project>,
    onChoose: (Long?) -> Unit,
) {
    if (projects.isEmpty()) return
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
        Text(
            text = stringResource(R.string.dialog_quickAdd_projectSuggestionHint),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            for (project in projects) {
                AssistChip(
                    onClick = { onChoose(project.localId) },
                    label = { Text(project.title) },
                    modifier = Modifier.semantics { contentDescription = project.title },
                )
            }
        }
    }
}

/** Where the task is going, and the way to change it. */
@Composable
private fun DestinationRow(
    projectTitle: String?,
    onPick: () -> Unit,
) {
    val name = projectTitle ?: stringResource(R.string.nav_inbox)
    AssistChip(
        onClick = onPick,
        label = { Text(name) },
        leadingIcon = {
            Icon(
                imageVector = if (projectTitle == null) Icons.Outlined.Inbox else Icons.Outlined.Folder,
                contentDescription = null,
            )
        },
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp).semantics { contentDescription = name },
    )
}

/** Today, tomorrow, or a day off the calendar. Tapping the day already set clears it. */
@OptIn(ExperimentalLayoutApi::class, ExperimentalMaterial3Api::class)
@Composable
private fun DueDateRow(
    dueDate: LocalDate?,
    today: LocalDate,
    onPick: (LocalDate?) -> Unit,
) {
    var picking by remember { mutableStateOf(false) }
    val tomorrow = today.plusDays(1)
    val custom = dueDate?.takeIf { it != today && it != tomorrow }

    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
        Text(
            text = stringResource(R.string.dialog_quickAdd_dueDateAriaLabel),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            DayChip(stringResource(R.string.common_today), dueDate == today) { onPick(today) }
            DayChip(stringResource(R.string.common_tomorrow), dueDate == tomorrow) { onPick(tomorrow) }
            DayChip(
                text =
                    custom?.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
                        ?: stringResource(R.string.dialog_quickAdd_pickDateTitle),
                selected = custom != null,
            ) { picking = true }
        }
    }

    if (!picking) return
    // The picker speaks in UTC midnights, so the day is read back at that offset
    // and only then handed on, to a caller that knows the user's own zone.
    val state =
        rememberDatePickerState(
            initialSelectedDateMillis = dueDate?.atStartOfDay(ZoneOffset.UTC)?.toInstant()?.toEpochMilli(),
        )
    DatePickerDialog(
        onDismissRequest = { picking = false },
        confirmButton = {
            TextButton(
                onClick = {
                    picking = false
                    val picked = state.selectedDateMillis ?: return@TextButton
                    onPick(Instant.ofEpochMilli(picked).atZone(ZoneOffset.UTC).toLocalDate())
                },
            ) { Text(stringResource(R.string.common_save)) }
        },
        dismissButton = {
            TextButton(onClick = { picking = false }) { Text(stringResource(R.string.common_cancel)) }
        },
    ) {
        DatePicker(state = state)
    }
}

@Composable
private fun DayChip(
    text: String,
    selected: Boolean,
    onClick: () -> Unit,
) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        label = { Text(text) },
        modifier = Modifier.semantics { contentDescription = text },
    )
}

/**
 * Where the task can go.
 *
 * The inbox leads, because it is the answer for anything not yet decided. The
 * places this device filed work into lately come next, lifted out of the main
 * list so nothing is offered twice, and everything else follows in the order the
 * project list is kept in.
 */
@Composable
private fun ProjectPicker(
    state: QuickAddUiState,
    callbacks: QuickAddCallbacks,
    modifier: Modifier = Modifier,
) {
    val searchLabel = stringResource(R.string.dialog_quickAdd_searchProjectsPlaceholder)
    Column(modifier = modifier.fillMaxWidth()) {
        OutlinedTextField(
            value = state.projectQuery,
            onValueChange = callbacks.onProjectQuery,
            placeholder = { Text(searchLabel) },
            modifier =
                Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 8.dp)
                    .semantics { contentDescription = searchLabel },
        )
        LazyColumn(
            modifier = Modifier.fillMaxWidth().heightIn(max = 360.dp),
            contentPadding = PaddingValues(bottom = 24.dp),
        ) {
            item {
                PickerRow(title = stringResource(R.string.nav_inbox)) { callbacks.onChooseProject(null) }
            }
            if (state.recentProjects.isNotEmpty()) {
                item { PickerHeading(stringResource(R.string.native_quickAdd_recentProjects)) }
                items(state.recentProjects, key = { "recent-${it.localId}" }) { project ->
                    PickerRow(project.title) { callbacks.onChooseProject(project.localId) }
                }
                item { HorizontalDivider() }
            }
            items(state.otherProjects, key = { it.localId }) { project ->
                PickerRow(project.title) { callbacks.onChooseProject(project.localId) }
            }
            if (state.recentProjects.isEmpty() && state.otherProjects.isEmpty()) {
                item { PickerHeading(stringResource(R.string.dialog_quickAdd_noMatches)) }
            }
        }
        TextButton(
            onClick = { callbacks.onPickingProject(false) },
            modifier = Modifier.align(Alignment.End).padding(horizontal = 8.dp, vertical = 4.dp),
        ) { Text(stringResource(R.string.common_cancel)) }
    }
}

@Composable
private fun PickerHeading(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

@Composable
private fun PickerRow(
    title: String,
    onClick: () -> Unit,
) {
    ListItem(
        headlineContent = { Text(title) },
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).semantics { contentDescription = title },
    )
}

/** How much of the accent the auto-label chips are filled with. The web's `bg-primary/5`. */
private const val AUTO_LABEL_TINT_ALPHA = 0.08f

/** And how much their outline keeps. The web's `border-primary/40`. */
private const val AUTO_LABEL_BORDER_ALPHA = 0.4f
