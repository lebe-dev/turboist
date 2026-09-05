package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.outlined.Circle
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
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
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.TaskRelationGroup
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.BlockerRef
import ru.tinyops.turboist.nativeapp.tasks.MoveProject
import ru.tinyops.turboist.nativeapp.tasks.TaskAddress
import ru.tinyops.turboist.nativeapp.tasks.TaskDetailPresenter
import ru.tinyops.turboist.nativeapp.tasks.TaskDetailUiState
import ru.tinyops.turboist.nativeapp.tasks.TaskDetailViewModel
import ru.tinyops.turboist.nativeapp.tasks.TaskListMessage
import ru.tinyops.turboist.nativeapp.tasks.deadlineDate
import ru.tinyops.turboist.nativeapp.tasks.deadlineTime
import ru.tinyops.turboist.nativeapp.tasks.dueDate
import ru.tinyops.turboist.nativeapp.tasks.dueTime
import ru.tinyops.turboist.nativeapp.ui.markdown.Markdown
import ru.tinyops.turboist.nativeapp.ui.markdown.MarkdownText
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId

/** Everything the detail screen can be asked to do, gathered into one object. */
data class TaskDetailCallbacks(
    val onBack: () -> Unit,
    val onRefresh: () -> Unit,
    val onToggleComplete: () -> Unit,
    val onRename: (String) -> Unit,
    val onDescribe: (String) -> Unit,
    val onPriority: (Priority) -> Unit,
    val onDayPart: (DayPart) -> Unit,
    val onPlanState: (PlanState) -> Unit,
    val onDueDate: (LocalDate?) -> Unit,
    val onDueTime: (LocalTime?) -> Unit,
    val onDeadlineDate: (LocalDate?) -> Unit,
    val onDeadlineTime: (LocalTime?) -> Unit,
    val onRecurrence: (String?) -> Unit,
    val onLabels: (List<String>) -> Unit,
    val onComplex: (Boolean) -> Unit,
    val onPrivate: (Boolean) -> Unit,
    val onTogglePin: () -> Unit,
    val onCancelTask: () -> Unit,
    val onDuplicate: () -> Unit,
    val onMove: (TaskDestination) -> Unit,
    val onDelete: () -> Unit,
    val onAddSubtask: (String) -> Unit,
    val onToggleSubtask: (Task) -> Unit,
    val onSearchRelations: (String) -> Unit,
    val onAddRelation: (Long, TaskRelationGroup) -> Unit,
    val onRemoveRelation: (Long) -> Unit,
    val onOpenTask: (Long) -> Unit,
    /** Splits the task into the titles typed into the outline. */
    val onDecompose: (List<String>) -> Unit = {},
    /** Saves the task and its subtree as a reusable template. */
    val onCreateTemplate: () -> Unit = {},
)

/**
 * One task, whole.
 *
 * Every field on this screen is editable with no network: an edit is applied to
 * the replica and queued in the same transaction, so the screen is correct the
 * moment the finger leaves it and stays correct until the queue drains, whenever
 * that turns out to be. Nothing here waits for an answer and nothing here can
 * fail for want of a connection.
 *
 * The one thing that is not optimistic is a refusal. A task something still
 * blocks cannot be completed, and the control says so before it is tapped rather
 * than after — which is the whole reason the rule is repeated on the device.
 */
@Composable
fun TaskDetailScreen(
    state: TaskDetailUiState,
    zone: ZoneId,
    today: LocalDate,
    moveOptions: List<MoveProject>,
    messages: Flow<TaskListMessage>,
    callbacks: TaskDetailCallbacks,
    modifier: Modifier = Modifier,
) {
    val snackbars = remember { SnackbarHostState() }
    val blocked = stringResource(R.string.task_toast_blockedCannotComplete)
    val linkExists = stringResource(R.string.native_task_relationExists)
    val linkCycle = stringResource(R.string.native_task_relationCycle)
    val failed = stringResource(R.string.task_toast_failedUpdate)
    val templateCreated = stringResource(R.string.task_actions_createTemplateSuccess)
    val templateFailed = stringResource(R.string.task_actions_createTemplateFailed)
    val splitFailed = stringResource(R.string.task_toast_failedDecompose)

    LaunchedEffect(messages, blocked, linkExists, linkCycle, failed) {
        messages.collect { message ->
            snackbars.showSnackbar(
                when (message) {
                    TaskListMessage.BLOCKED -> blocked
                    TaskListMessage.RELATION_EXISTS -> linkExists
                    TaskListMessage.RELATION_CYCLE -> linkCycle
                    TaskListMessage.TEMPLATE_CREATED -> templateCreated
                    TaskListMessage.TEMPLATE_FAILED -> templateFailed
                    TaskListMessage.DECOMPOSE_FAILED -> splitFailed
                    TaskListMessage.FAILED -> failed
                },
            )
        }
    }

    Box(modifier = modifier.fillMaxSize()) {
        TaskDetailBody(state, zone, today, moveOptions, callbacks)
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun TaskDetailBody(
    state: TaskDetailUiState,
    zone: ZoneId,
    today: LocalDate,
    moveOptions: List<MoveProject>,
    callbacks: TaskDetailCallbacks,
) {
    var moving by remember { mutableStateOf(false) }

    PullToRefreshBox(
        isRefreshing = state.refreshing,
        onRefresh = callbacks.onRefresh,
        modifier = Modifier.fillMaxSize(),
    ) {
        Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState())) {
            BackRow(callbacks.onBack)
            val task = state.task
            if (task == null) {
                // Either the link named a task this device has never replicated,
                // or the task is gone. Both read the same to the user, and both
                // leave nothing to show.
                if (state.missing) {
                    Text(
                        text = stringResource(R.string.page_task_notFound),
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier.padding(16.dp),
                    )
                }
                return@Column
            }

            TitleBlock(task, state.blocked, callbacks)
            if (state.blockers.isNotEmpty()) BlockedNotice(state.blockers, callbacks.onOpenTask)
            DescriptionBlock(task.description, callbacks.onDescribe)
            HorizontalDivider()
            PlacementRow(state, onOpenMove = { moving = true })
            DateBlock(task, zone, callbacks)
            RecurrenceField(
                rule = task.recurrenceRule,
                dueAt = task.dueAt,
                from = today.atStartOfDay(zone).toInstant().toEpochMilli(),
                zone = zone,
                onChange = callbacks.onRecurrence,
            )
            PrioritySelector(task.priority, callbacks.onPriority, locked = state.priorityLocked)
            DayPartSelector(task.dayPart, callbacks.onDayPart)
            PlanSelector(task.planState, callbacks.onPlanState)
            LabelSelector(state.knownLabels, task.labels.map { it.name }, callbacks.onLabels)
            FlagRows(task, callbacks)
            HorizontalDivider()
            FactRows(task, zone)
            HorizontalDivider()
            TaskDetailActionRow(
                task = task,
                onTogglePin = callbacks.onTogglePin,
                onCancelTask = callbacks.onCancelTask,
                onDuplicate = callbacks.onDuplicate,
                onOpenMove = { moving = true },
                onDelete = callbacks.onDelete,
                // A task with work under it cannot be split: the work would have
                // nowhere to go, and the server refuses it for the same reason.
                canDecompose = state.openSubtasks.isEmpty() && state.doneSubtasks.isEmpty(),
                onDecompose = callbacks.onDecompose,
                onCreateTemplate = callbacks.onCreateTemplate,
            )
            HorizontalDivider()
            TaskDetailRelations(
                relations = state.relations,
                candidates = state.relationCandidates,
                onSearch = callbacks.onSearchRelations,
                onAdd = callbacks.onAddRelation,
                onRemove = callbacks.onRemoveRelation,
                onOpen = callbacks.onOpenTask,
            )
            HorizontalDivider()
            TaskDetailSubtasks(
                open = state.openSubtasks,
                done = state.doneSubtasks,
                inInbox = state.placement.inInbox,
                zone = zone,
                today = today,
                onToggleComplete = callbacks.onToggleSubtask,
                onOpen = { callbacks.onOpenTask(it.localId) },
                onAdd = callbacks.onAddSubtask,
            )
        }
    }

    if (!moving) return
    TaskMoveSheet(
        projects = moveOptions,
        onPick = { destination ->
            moving = false
            callbacks.onMove(destination)
        },
        onDismiss = { moving = false },
    )
}

@Composable
private fun BackRow(onBack: () -> Unit) {
    IconButton(onClick = onBack, modifier = Modifier.padding(start = 4.dp)) {
        Icon(
            imageVector = Icons.AutoMirrored.Filled.ArrowBack,
            contentDescription = stringResource(R.string.common_back),
        )
    }
}

/**
 * The tick control and the title, side by side.
 *
 * A blocked task shows a padlock instead of an empty circle and cannot be
 * ticked, which is the same swap every list row makes: a greyed-out circle still
 * reads as "a circle you may tick later", and the padlock says what is true.
 */
@Composable
private fun TitleBlock(
    task: Task,
    blocked: Boolean,
    callbacks: TaskDetailCallbacks,
) {
    val completed = task.status == TaskStatus.COMPLETED
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp),
        verticalAlignment = Alignment.Top,
    ) {
        IconButton(onClick = callbacks.onToggleComplete, enabled = !blocked) {
            when {
                blocked ->
                    Icon(
                        imageVector = Icons.Outlined.Lock,
                        contentDescription = stringResource(R.string.task_blockedTooltip),
                        tint = priorityTint(task.priority),
                    )

                completed ->
                    Icon(
                        imageVector = Icons.Filled.CheckCircle,
                        contentDescription = stringResource(R.string.task_markIncomplete),
                        tint = MaterialTheme.colorScheme.outline,
                    )

                else ->
                    Icon(
                        imageVector = Icons.Outlined.Circle,
                        contentDescription = stringResource(R.string.task_markComplete),
                        tint = priorityTint(task.priority),
                    )
            }
        }
        CommittedTextField(
            value = task.title,
            label = stringResource(R.string.page_task_namePlaceholder),
            singleLine = true,
            strikeThrough = completed,
            onCommit = callbacks.onRename,
            modifier = Modifier.weight(1f).padding(top = 4.dp),
        )
    }
}

/**
 * The work standing in the way, named and reachable.
 *
 * A count would say the task cannot be finished; the names say what to do about
 * it, and tapping one goes there.
 */
@Composable
private fun BlockedNotice(
    blockers: List<BlockerRef>,
    onOpenTask: (Long) -> Unit,
) {
    Surface(
        color = MaterialTheme.colorScheme.errorContainer,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
    ) {
        Column(modifier = Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                Icon(
                    imageVector = Icons.Outlined.Lock,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onErrorContainer,
                    modifier = Modifier.size(16.dp),
                )
                Text(
                    text = stringResource(R.string.page_task_relation_blockedBy),
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.onErrorContainer,
                )
            }
            for (blocker in blockers) {
                Text(
                    text = blocker.title,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onErrorContainer,
                    modifier = Modifier.fillMaxWidth().clickable { onOpenTask(blocker.taskLocalId) },
                )
            }
        }
    }
}

/**
 * The description: shown as the formatted text it describes, edited as the plain
 * text it is.
 *
 * The two are never on screen at once, because the markup only makes sense while
 * it is being read and only gets in the way while it is being written. Text with
 * no markup in it is left as typed rather than run through a renderer that would
 * silently reflow the author's own spacing.
 */
@Composable
private fun DescriptionBlock(
    description: String,
    onCommit: (String) -> Unit,
) {
    var editing by remember(description) { mutableStateOf(false) }
    if (editing) {
        CommittedTextField(
            value = description,
            label = stringResource(R.string.page_task_descriptionPlaceholder),
            singleLine = false,
            strikeThrough = false,
            onCommit = { text ->
                editing = false
                onCommit(text)
            },
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
            autoFocus = true,
        )
        return
    }
    Column(
        modifier =
            Modifier
                .fillMaxWidth()
                .clickable { editing = true }
                .padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        when {
            description.isEmpty() ->
                Text(
                    text = stringResource(R.string.native_task_noDescription),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )

            Markdown.hasContent(description) -> MarkdownText(description)

            else -> Text(text = description, style = MaterialTheme.typography.bodyMedium)
        }
    }
}

/** Where the task lives, and the way to move it. */
@Composable
private fun PlacementRow(
    state: TaskDetailUiState,
    onOpenMove: () -> Unit,
) {
    val placement = state.placement
    val value =
        when {
            placement.parentTitle != null -> placement.parentTitle
            placement.sectionTitle != null && placement.projectTitle != null ->
                placement.projectTitle + " · " + placement.sectionTitle

            placement.projectTitle != null -> placement.projectTitle
            placement.contextName != null -> placement.contextName
            placement.inInbox -> stringResource(R.string.nav_inbox)
            else -> stringResource(R.string.native_task_unset)
        }
    DetailRow(
        label = stringResource(R.string.native_task_location),
        value = value,
        onClick = onOpenMove,
    )
}

/** The two dates a task can carry, each with an optional time of day. */
@Composable
private fun DateBlock(
    task: Task,
    zone: ZoneId,
    callbacks: TaskDetailCallbacks,
) {
    val due = task.dueDate(zone)
    DateField(
        label = stringResource(R.string.page_task_date),
        date = due,
        clearLabel = stringResource(R.string.page_task_clearDate),
        onPick = callbacks.onDueDate,
    )
    if (due != null) {
        TimeField(
            label = stringResource(R.string.native_task_time),
            time = task.dueTime(zone),
            onPick = callbacks.onDueTime,
        )
    }
    val deadline = task.deadlineDate(zone)
    DateField(
        label = stringResource(R.string.native_task_deadline),
        date = deadline,
        clearLabel = stringResource(R.string.task_actions_clearDate),
        onPick = callbacks.onDeadlineDate,
    )
    if (deadline != null) {
        TimeField(
            label = stringResource(R.string.native_task_deadline) + " · " + stringResource(R.string.native_task_time),
            time = task.deadlineTime(zone),
            onPick = callbacks.onDeadlineTime,
        )
    }
}

/** The two marks a task carries about itself. */
@Composable
private fun FlagRows(
    task: Task,
    callbacks: TaskDetailCallbacks,
) {
    DetailSwitchRow(
        label = stringResource(R.string.task_complexMarker),
        checked = task.isComplex,
        onCheckedChange = callbacks.onComplex,
    )
    DetailSwitchRow(
        label = stringResource(R.string.common_privateMarker),
        checked = task.isPrivate,
        onCheckedChange = callbacks.onPrivate,
    )
    // Pinning is a write of its own rather than a field edit — the shelf has a
    // cap, and the write path is what refuses a pin over it.
    DetailSwitchRow(
        label = stringResource(R.string.nav_pinned),
        checked = task.isPinned,
        onCheckedChange = { callbacks.onTogglePin() },
    )
}

/**
 * What the task has been through: how often it has been put off, and when it was
 * written down, last changed and finished.
 *
 * All of it is read-only. A postpone count that could be edited would stop being
 * a record of anything, and the timestamps belong to the server.
 */
@Composable
private fun FactRows(
    task: Task,
    zone: ZoneId,
) {
    DetailRow(
        label = stringResource(R.string.native_task_postponed),
        value = task.postponeCount.toString(),
    )
    DetailRow(label = stringResource(R.string.native_task_status), value = statusLabel(task.status))
    DetailRow(label = stringResource(R.string.native_task_created), value = momentLabel(task.createdAt, zone))
    DetailRow(label = stringResource(R.string.native_task_updated), value = momentLabel(task.updatedAt, zone))
    if (task.completedAt != null) {
        DetailRow(
            label = stringResource(R.string.native_task_completedAt),
            value = momentLabel(task.completedAt, zone),
        )
    }
}

/** Where the task stands: still open, finished, or closed without being done. */
@Composable
private fun statusLabel(status: TaskStatus): String =
    when (status) {
        TaskStatus.OPEN -> stringResource(R.string.native_task_statusOpen)
        TaskStatus.COMPLETED -> stringResource(R.string.native_task_statusCompleted)
        TaskStatus.CANCELLED -> stringResource(R.string.native_task_statusCancelled)
        TaskStatus.UNKNOWN -> stringResource(R.string.native_task_unset)
    }

/**
 * A text field that reports its content once, when the user is done with it.
 *
 * Reporting on every keystroke would turn one sentence into one queued write per
 * character; reporting a value that did not change would queue a write for
 * merely having looked at the field. So the commit happens when focus leaves or
 * when the keyboard's action is taken, and what it reports is compared with what
 * the task already says before anything is sent.
 */
@Composable
private fun CommittedTextField(
    value: String,
    label: String,
    singleLine: Boolean,
    strikeThrough: Boolean,
    onCommit: (String) -> Unit,
    modifier: Modifier = Modifier,
    autoFocus: Boolean = false,
) {
    var text by remember(value) { mutableStateOf(value) }
    var hadFocus by remember { mutableStateOf(false) }
    val focusRequester = remember { FocusRequester() }
    // A field the user asked for by tapping the text it replaces should be ready
    // to type into; one that is simply on screen should not steal the keyboard.
    LaunchedEffect(autoFocus) { if (autoFocus) focusRequester.requestFocus() }
    OutlinedTextField(
        value = text,
        onValueChange = { text = it },
        label = { Text(label) },
        singleLine = singleLine,
        textStyle =
            MaterialTheme.typography.bodyLarge.copy(
                textDecoration = if (strikeThrough) TextDecoration.LineThrough else null,
            ),
        keyboardOptions = KeyboardOptions(imeAction = if (singleLine) ImeAction.Done else ImeAction.Default),
        keyboardActions = KeyboardActions(onDone = { onCommit(text) }),
        modifier =
            modifier.focusRequester(focusRequester).onFocusChanged { focus ->
                if (focus.isFocused) {
                    hadFocus = true
                } else if (hadFocus) {
                    hadFocus = false
                    onCommit(text)
                }
            },
    )
}

/**
 * The screen as the app wires it: a view model behind it, and the two things
 * only the navigation graph can supply — how to open another task, and how to
 * leave.
 */
@Composable
fun TaskDetailScreen(
    address: TaskAddress,
    onOpenTask: (Long) -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: TaskDetailViewModel = hiltViewModel(),
) {
    LaunchedEffect(address) { viewModel.open(address) }

    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    val today by viewModel.today.collectAsStateWithLifecycle()
    val moveOptions by viewModel.moveOptions.collectAsStateWithLifecycle()
    val zone = viewModel.zone

    // A deleted task has nothing left to show, so the screen leaves rather than
    // sitting on an empty frame the user has to back out of themselves.
    LaunchedEffect(presenter) { presenter.deleted.collect { onBack() } }

    TaskDetailScreen(
        state = state,
        zone = zone,
        today = today,
        moveOptions = moveOptions,
        messages = presenter.messages,
        callbacks = detailCallbacks(presenter, zone, onOpenTask, onBack),
        modifier = modifier,
    )
}

/** Wires the screen's gestures onto the presenter behind it. */
@Composable
private fun detailCallbacks(
    presenter: TaskDetailPresenter,
    zone: ZoneId,
    onOpenTask: (Long) -> Unit,
    onBack: () -> Unit,
): TaskDetailCallbacks =
    remember(presenter, zone, onOpenTask, onBack) {
        TaskDetailCallbacks(
            onBack = onBack,
            onRefresh = presenter::refresh,
            onToggleComplete = presenter::toggleComplete,
            onRename = presenter::rename,
            onDescribe = presenter::describe,
            onPriority = presenter::setPriority,
            onDayPart = presenter::setDayPart,
            onPlanState = presenter::setPlanState,
            onDueDate = { presenter.setDueDate(it, zone) },
            onDueTime = { presenter.setDueTime(it, zone) },
            onDeadlineDate = { presenter.setDeadlineDate(it, zone) },
            onDeadlineTime = { presenter.setDeadlineTime(it, zone) },
            onRecurrence = presenter::setRecurrence,
            onLabels = presenter::setLabels,
            onComplex = presenter::setComplex,
            onPrivate = presenter::setPrivate,
            onTogglePin = presenter::togglePin,
            onCancelTask = presenter::cancel,
            onDuplicate = presenter::duplicate,
            onMove = presenter::move,
            onDelete = presenter::delete,
            onAddSubtask = presenter::addSubtask,
            onToggleSubtask = presenter::toggleSubtaskComplete,
            onSearchRelations = presenter::searchRelationCandidates,
            onAddRelation = presenter::addRelation,
            onRemoveRelation = presenter::removeRelation,
            onOpenTask = onOpenTask,
            onDecompose = presenter::decompose,
            onCreateTemplate = presenter::createTemplate,
        )
    }
