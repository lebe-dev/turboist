package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.BulkAction
import ru.tinyops.turboist.nativeapp.tasks.BulkDestinations
import ru.tinyops.turboist.nativeapp.tasks.BulkOutcome
import ru.tinyops.turboist.nativeapp.tasks.TaskListMessage
import ru.tinyops.turboist.nativeapp.tasks.TaskListPresenter
import ru.tinyops.turboist.nativeapp.tasks.TaskListUiState
import ru.tinyops.turboist.nativeapp.tasks.TaskListViewModel
import java.time.LocalDate
import java.time.ZoneId

/**
 * Which sheet, if any, a selection has opened.
 *
 * Held as one value rather than as two flags because the two are exclusive and
 * belong to the same gesture: a selection is being sent somewhere, or being
 * gathered under a new task, never both.
 */
private enum class SelectionSheet { NONE, MOVE, GROUP_TITLE, GROUP_DESTINATION }

/**
 * A task list screen: the rows, plus the one place a screen talks back.
 *
 * Everything a list does happens without waiting for the server, so the only
 * thing left to say afterwards is the exception — a change that did not take.
 * That is a snackbar over the list rather than a dialog or an inline error,
 * because the list is still correct and still usable: the row simply stayed as
 * it was.
 *
 * A selection action is the other thing worth a sentence, and for the opposite
 * reason: it usually works, and the tasks it worked on leave the screen, so the
 * count is the only evidence left that the gesture landed.
 */
@Composable
fun TaskListScreen(
    state: TaskListUiState,
    zone: ZoneId,
    today: LocalDate,
    empty: EmptyListText?,
    messages: Flow<TaskListMessage>,
    callbacks: TaskListCallbacks,
    modifier: Modifier = Modifier,
    bulkOutcomes: Flow<BulkOutcome>? = null,
) {
    val snackbars = remember { SnackbarHostState() }
    val blocked = stringResource(R.string.task_toast_blockedCannotComplete)
    val failed = stringResource(R.string.task_toast_failedUpdate)

    LaunchedEffect(messages, blocked, failed) {
        messages.collect { message ->
            snackbars.showSnackbar(
                when (message) {
                    TaskListMessage.BLOCKED -> blocked
                    // A list does not link tasks, make templates out of them or
                    // split them, so none of those answers can reach this screen.
                    // They are still answered rather than left to fall through as
                    // nothing at all.
                    TaskListMessage.RELATION_EXISTS,
                    TaskListMessage.RELATION_CYCLE,
                    TaskListMessage.TEMPLATE_CREATED,
                    TaskListMessage.TEMPLATE_FAILED,
                    TaskListMessage.DECOMPOSE_FAILED,
                    -> failed

                    TaskListMessage.FAILED -> failed
                },
            )
        }
    }
    BulkOutcomeSnackbars(bulkOutcomes, snackbars)

    Box(modifier = modifier.fillMaxSize()) {
        TaskList(
            state = state,
            zone = zone,
            today = today,
            empty = empty,
            callbacks = callbacks,
        )
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }
}

/**
 * Turns what a selection action did into a sentence.
 *
 * A completion that left some of the selection alone gets the fuller sentence,
 * naming how many are still waiting on something. Saying only "8 completed"
 * after ten were picked would leave the user counting rows to find out what
 * happened to the other two.
 */
@Composable
private fun BulkOutcomeSnackbars(
    outcomes: Flow<BulkOutcome>?,
    snackbars: SnackbarHostState,
) {
    if (outcomes == null) return
    val wording = bulkWording()
    LaunchedEffect(outcomes, wording) {
        outcomes.collect { outcome -> snackbars.showSnackbar(wording(outcome)) }
    }
}

/**
 * The sentence for each outcome, resolved before anything happens.
 *
 * The resources are read here rather than inside the collector because a
 * collector is not a composition and cannot read them.
 */
@Composable
private fun bulkWording(): (BulkOutcome) -> String {
    val resources = LocalContext.current.resources
    return { outcome ->
        // The counts are handed over as text: the shared wording carries no type
        // for its values, so every generated placeholder is a string.
        when {
            outcome.action == BulkAction.COMPLETED && outcome.leftBlocked > 0 ->
                resources.getString(
                    R.string.task_toast_bulkCompletedBlocked,
                    outcome.changed.toString(),
                    outcome.leftBlocked.toString(),
                )

            outcome.action == BulkAction.PRIORITISED && outcome.leftLocked > 0 ->
                resources.getString(
                    R.string.task_toast_bulkPrioritySetPartial,
                    outcome.changed.toString(),
                    outcome.leftLocked.toString(),
                )

            else -> resources.getString(wordingFor(outcome.action), outcome.changed.toString())
        }
    }
}

/** Which sentence names each action, with the count as its one value. */
@StringRes
private fun wordingFor(action: BulkAction): Int =
    when (action) {
        BulkAction.COMPLETED -> R.string.task_toast_bulkCompleted
        BulkAction.MOVED -> R.string.task_toast_bulkMoved
        BulkAction.PRIORITISED -> R.string.task_toast_bulkPrioritySet
        BulkAction.PLANNED -> R.string.task_toast_bulkPlanned
        BulkAction.PARKED -> R.string.task_toast_bulkParked
        BulkAction.DELETED -> R.string.task_toast_bulkDeleted
        BulkAction.GROUPED -> R.string.task_toast_grouped
    }

/**
 * The same screen, driven by a view model.
 *
 * The stateless form above is the one that is exercised in tests; this is the
 * wiring the app uses, and it holds nothing of its own except which sheet a
 * selection has opened, which is a fact about this screen rather than about the
 * work.
 *
 * A task with no server id yet cannot be opened: the detail screen is addressed
 * the way the web client addresses it, by server id, and a row created offline
 * has none until the queue drains. Tapping it does nothing rather than opening
 * something empty.
 */
@Composable
fun TaskListScreen(
    viewModel: TaskListViewModel,
    empty: EmptyListText?,
    onOpenTask: (Task) -> Unit,
    modifier: Modifier = Modifier,
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    val today by viewModel.today.collectAsStateWithLifecycle()
    val destinations by presenter.destinations.collectAsStateWithLifecycle()
    val sheet = remember { mutableStateOf(SelectionSheet.NONE) }
    val groupTitle = remember { mutableStateOf("") }
    val onMoveRequested = remember { { sheet.value = SelectionSheet.MOVE } }
    val onGroupRequested = remember { { sheet.value = SelectionSheet.GROUP_TITLE } }

    TaskListScreen(
        state = state,
        zone = viewModel.zone,
        today = today,
        empty = empty,
        messages = presenter.messages,
        callbacks = presenterCallbacks(presenter, onOpenTask, onMoveRequested, onGroupRequested),
        modifier = modifier,
        bulkOutcomes = presenter.bulkOutcomes,
    )
    SelectionSheets(
        sheet = sheet.value,
        selectedCount = state.selected.size,
        destinations = destinations,
        onDismiss = { sheet.value = SelectionSheet.NONE },
        onTitleChosen = { chosen ->
            groupTitle.value = chosen
            sheet.value = SelectionSheet.GROUP_DESTINATION
        },
        onMoveTo = { destination ->
            sheet.value = SelectionSheet.NONE
            presenter.moveSelected(destination)
        },
        onGroupInto = { destination ->
            sheet.value = SelectionSheet.NONE
            presenter.groupSelected(groupTitle.value, destination)
        },
    )
}

/**
 * The sheets a selection action opens.
 *
 * Grouping asks two questions in turn — what the new task is called, then where
 * it goes — rather than one sheet holding both. The second question has a long
 * answer list, and a text field above a scrolling list on a phone leaves the
 * field behind the keyboard.
 */
@Composable
private fun SelectionSheets(
    sheet: SelectionSheet,
    selectedCount: Int,
    destinations: BulkDestinations,
    onDismiss: () -> Unit,
    onTitleChosen: (String) -> Unit,
    onMoveTo: (TaskDestination) -> Unit,
    onGroupInto: (TaskDestination) -> Unit,
) {
    when (sheet) {
        SelectionSheet.NONE -> Unit
        SelectionSheet.MOVE ->
            BulkDestinationSheet(
                title = stringResource(R.string.dialog_bulkMove_title),
                destinations = destinations,
                offerInbox = true,
                onPick = onMoveTo,
                onDismiss = onDismiss,
            )

        SelectionSheet.GROUP_TITLE ->
            GroupTasksSheet(
                count = selectedCount,
                onConfirm = onTitleChosen,
                onDismiss = onDismiss,
            )

        SelectionSheet.GROUP_DESTINATION ->
            BulkDestinationSheet(
                title = stringResource(R.string.selection_group_destinationTitle),
                destinations = destinations,
                // A group is structure, and the inbox holds none: the server
                // refuses a parent filed there, so it is never offered.
                offerInbox = false,
                onPick = onGroupInto,
                onDismiss = onDismiss,
            )
    }
}

/** Wires a row's gestures, and the selection bar's, onto the presenter behind the screen. */
@Composable
private fun presenterCallbacks(
    presenter: TaskListPresenter,
    onOpenTask: (Task) -> Unit,
    onMoveRequested: () -> Unit,
    onGroupRequested: () -> Unit,
): TaskListCallbacks =
    remember(presenter, onOpenTask, onMoveRequested, onGroupRequested) {
        TaskListCallbacks(
            onToggleComplete = presenter::toggleComplete,
            onOpen = onOpenTask,
            onPark = presenter::park,
            onPlanForWeek = presenter::planForWeek,
            onStartSelection = { presenter.startSelection(it.localId) },
            onSelectToggle = { presenter.toggleSelection(it.localId) },
            onClearSelection = presenter::clearSelection,
            onRefresh = presenter::refresh,
            onSelectAllInSection = presenter::toggleSelectAll,
            selectionActions =
                if (!presenter.offersBulkActions) {
                    null
                } else {
                    SelectionActions(
                        onComplete = presenter::completeSelected,
                        onMove = onMoveRequested,
                        onGroup = onGroupRequested,
                        onPriority = presenter::prioritiseSelected,
                        onPlan = presenter::planSelected,
                        onDelete = presenter::deleteSelected,
                    )
                },
        )
    }
