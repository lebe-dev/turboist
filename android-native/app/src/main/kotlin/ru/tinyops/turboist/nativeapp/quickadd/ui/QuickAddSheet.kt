package ru.tinyops.turboist.nativeapp.quickadd.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.outlined.AutoAwesome
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.InputChip
import androidx.compose.material3.InputChipDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.SheetValue
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldColors
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddMessage
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddPresenter
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddRequest
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddUiState
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddViewModel
import java.time.LocalDate

/** Every gesture the capture sheet can make. */
data class QuickAddCallbacks(
    val onDismiss: () -> Unit = {},
    val onDiscard: () -> Unit = {},
    val onKeepEditing: () -> Unit = {},
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
    showButton: Boolean = true,
    projectLocalId: Long? = null,
    onToday: Boolean = false,
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
        // The button opens on the project on screen, if any: a task written down
        // while looking at a project belongs to it far more often than to the inbox.
        onOpen = { presenter.open(QuickAddRequest(projectLocalId = projectLocalId, todayProjects = onToday)) },
        modifier = modifier,
        showButton = showButton,
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
    showButton: Boolean = true,
) {
    val snackbars = remember { SnackbarHostState() }
    val addedToInbox = stringResource(R.string.task_toast_addedToInbox)
    val addedToProject = stringResource(R.string.task_toast_addedToProject)
    val failed = stringResource(R.string.task_toast_failedAdd)
    val todayLocked = stringResource(R.string.task_toast_todayLocked)

    LaunchedEffect(messages, addedToInbox, addedToProject, failed, todayLocked) {
        messages.collect { message ->
            snackbars.showSnackbar(
                when (message) {
                    QuickAddMessage.ADDED_TO_INBOX -> addedToInbox
                    QuickAddMessage.ADDED_TO_PROJECT -> addedToProject
                    QuickAddMessage.FAILED -> failed
                    QuickAddMessage.TODAY_LOCKED -> todayLocked
                },
            )
        }
    }

    Box(modifier = modifier.fillMaxSize()) {
        // The button is drawn over whatever screen is on top, so a screen with a
        // control of its own in that corner asks for it to be left out. The sheet
        // is not conditional on it: a share from another app opens the sheet
        // without anyone having touched the button.
        if (showButton) {
            FloatingActionButton(
                onClick = onOpen,
                containerColor = MaterialTheme.colorScheme.primary,
                contentColor = MaterialTheme.colorScheme.onPrimary,
                modifier = Modifier.align(Alignment.BottomEnd).padding(16.dp),
            ) {
                Icon(
                    imageVector = Icons.Filled.Add,
                    contentDescription = stringResource(R.string.task_addTask),
                )
            }
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
            onDismiss = presenter::requestDismiss,
            onDiscard = presenter::dismiss,
            onKeepEditing = presenter::keepEditing,
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
    val scope = rememberCoroutineScope()
    val latestState by rememberUpdatedState(state)
    val latestCallbacks by rememberUpdatedState(callbacks)
    // A swipe or a tap outside would slide the sheet away before anyone could be
    // asked, so one that holds something refuses to go and asks instead. The lambda
    // has to stay the same one: the sheet's state is keyed on it.
    val confirmValueChange =
        remember {
            { target: SheetValue ->
                val refused = target == SheetValue.Hidden && latestState.holdsInput
                if (refused) latestCallbacks.onDismiss()
                !refused
            }
        }
    val sheetState =
        rememberModalBottomSheetState(
            skipPartiallyExpanded = true,
            confirmValueChange = confirmValueChange,
        )
    ModalBottomSheet(
        onDismissRequest = {
            callbacks.onDismiss()
            // The back gesture hides the sheet without asking the check above. If
            // the sheet is staying, it has to come back; if not, this is cancelled
            // along with the sheet.
            scope.launch { sheetState.show() }
        },
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
        ProjectPicker(state = state, today = today, callbacks = callbacks, modifier = modifier)
    } else {
        QuickAddFields(state = state, today = today, callbacks = callbacks, modifier = modifier)
    }
    if (state.confirmingDiscard) {
        DiscardDraftDialog(onDiscard = callbacks.onDiscard, onKeepEditing = callbacks.onKeepEditing)
    }
}

/** Asks before the sheet throws away what it holds. */
@Composable
private fun DiscardDraftDialog(
    onDiscard: () -> Unit,
    onKeepEditing: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onKeepEditing,
        title = { Text(stringResource(R.string.native_quickAdd_discardTitle)) },
        text = { Text(stringResource(R.string.native_quickAdd_discardBody)) },
        confirmButton = {
            TextButton(onClick = onDiscard) { Text(stringResource(R.string.native_quickAdd_discardConfirm)) }
        },
        dismissButton = {
            TextButton(onClick = onKeepEditing) { Text(stringResource(R.string.native_quickAdd_discardKeep)) }
        },
    )
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
    var panel by rememberSaveable { mutableStateOf<QuickAddPanel?>(null) }
    val keyboard = LocalSoftwareKeyboardController.current

    // The sheet is anchored to the bottom, so any change in its content height moves
    // its top edge. Chips that appear while typing (auto labels, project
    // suggestions) would make the whole sheet jump up and down, so the content
    // always takes the full height it is offered and grows inside it instead.
    Column(modifier = modifier.fillMaxWidth().fillMaxHeight()) {
        Column(
            modifier =
                Modifier
                    .weight(1f)
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(bottom = 16.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            // The title is the one thing the sheet is opened for, so it is written
            // large and without a box: a field that looks like a form asks to be
            // filled in, a line that looks like a heading asks to be written.
            TextField(
                value = state.draft.titles,
                onValueChange = callbacks.onTitles,
                placeholder = { Text(stringResource(R.string.dialog_quickAdd_titlePlaceholder)) },
                textStyle = MaterialTheme.typography.headlineSmall.copy(fontWeight = FontWeight.Medium),
                colors = borderlessFieldColors(),
                visualTransformation = HiddenTitleTail,
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 4.dp)
                        .focusRequester(focus)
                        .semantics { contentDescription = titleLabel },
            )

            TextField(
                value = state.draft.description,
                onValueChange = callbacks.onDescription,
                placeholder = { Text(stringResource(R.string.dialog_quickAdd_descriptionPlaceholder)) },
                textStyle = MaterialTheme.typography.bodyLarge,
                colors = borderlessFieldColors(),
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 4.dp)
                        .semantics { contentDescription = descriptionLabel },
            )

            AutoLabelChips(names = state.autoLabels, onReject = callbacks.onRejectAutoLabel)
            SuggestedProjectChips(projects = state.suggestedProjects, onChoose = callbacks.onChooseProject)

            // Where the task goes and when it is for are one decision while the
            // focus lock is on, so they sit in one card that explains the lock.
            PlacementCard(state = state, today = today, callbacks = callbacks)
        }

        // Priority, part of the day and labels are set less often than the title,
        // so they wait in a bar pinned above the keyboard: each shows its value and
        // opens its editor on a tap. The save button lives there too, so it never
        // scrolls out of reach.
        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
        AttributePanel(panel = panel, state = state, callbacks = callbacks, onClose = { panel = null })
        AttributeBar(
            state = state,
            open = panel,
            onToggle = { target ->
                if (panel != target) keyboard?.hide()
                panel = if (panel == target) null else target
            },
            onSubmit = callbacks.onSubmit,
            submitDescription = submitLabel(state.draft.titleLines.size),
        )
    }
}

/** The editors the bar above the keyboard can open, one at a time. */
internal enum class QuickAddPanel { PRIORITY, DAY_PART, LABELS }

/** A text field drawn as bare text on the sheet: no box, no underline. */
@Composable
private fun borderlessFieldColors(): TextFieldColors =
    TextFieldDefaults.colors(
        focusedContainerColor = Color.Transparent,
        unfocusedContainerColor = Color.Transparent,
        disabledContainerColor = Color.Transparent,
        focusedIndicatorColor = Color.Transparent,
        unfocusedIndicatorColor = Color.Transparent,
        disabledIndicatorColor = Color.Transparent,
    )

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

/** How much of the accent the auto-label chips are filled with. The web's `bg-primary/5`. */
private const val AUTO_LABEL_TINT_ALPHA = 0.08f

/** And how much their outline keeps. The web's `border-primary/40`. */
private const val AUTO_LABEL_BORDER_ALPHA = 0.4f
