package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.automirrored.outlined.Notes
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.PushPin
import androidx.compose.material.icons.outlined.CalendarMonth
import androidx.compose.material.icons.outlined.Circle
import androidx.compose.material.icons.outlined.Flag
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Layers
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.PushPin
import androidx.compose.material.icons.outlined.Repeat
import androidx.compose.material.icons.outlined.Splitscreen
import androidx.compose.material.icons.outlined.WbSunny
import androidx.compose.material3.AssistChip
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.InputChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
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
import ru.tinyops.turboist.nativeapp.tasks.RecurrenceWording
import ru.tinyops.turboist.nativeapp.tasks.TaskAddress
import ru.tinyops.turboist.nativeapp.tasks.TaskDetailPresenter
import ru.tinyops.turboist.nativeapp.tasks.TaskDetailUiState
import ru.tinyops.turboist.nativeapp.tasks.TaskDetailViewModel
import ru.tinyops.turboist.nativeapp.tasks.TaskListMessage
import ru.tinyops.turboist.nativeapp.tasks.TaskPlacement
import ru.tinyops.turboist.nativeapp.tasks.deadlineDate
import ru.tinyops.turboist.nativeapp.tasks.deadlineTime
import ru.tinyops.turboist.nativeapp.tasks.dueDate
import ru.tinyops.turboist.nativeapp.tasks.dueTime
import ru.tinyops.turboist.nativeapp.tasks.recurrenceWording
import ru.tinyops.turboist.nativeapp.ui.markdown.Markdown
import ru.tinyops.turboist.nativeapp.ui.markdown.MarkdownText
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
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

/** The field a sheet is currently open for, or nothing when none is. */
private enum class TaskField { DATE, DAY_PART, DEADLINE, REPEAT, PLAN, LABELS, ACTIONS, MOVE }

/**
 * One task, whole.
 *
 * Every field on this screen is editable with no network: an edit is applied to
 * the replica and queued in the same transaction, so the screen is correct the
 * moment the finger leaves it and stays correct until the queue drains, whenever
 * that turns out to be. Nothing here waits for an answer and nothing here can
 * fail for want of a connection.
 *
 * What the screen shows at rest is what the task *is*: where it lives, what it
 * is called, when it is due, how it is planned, what is under it. What can be
 * *changed about it* — a choice out of several, every label in the workspace,
 * the six things that can be done to the whole task — waits behind a row or the
 * overflow until it is asked for. A page that lays all of it out at once has
 * nothing to read, only things to operate.
 *
 * The one thing that is not optimistic is a refusal. A task something still
 * blocks cannot be completed, and the control says so before it is tapped rather
 * than after — which is the whole reason the rule is repeated on the device.
 *
 * [harpoon] is the jump-pair control, handed in by the graph: it navigates, and
 * where a destination leads is the graph's business rather than the screen's.
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
    harpoon: @Composable () -> Unit = {},
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

    TaskDetailBody(
        state = state,
        zone = zone,
        today = today,
        moveOptions = moveOptions,
        callbacks = callbacks,
        snackbars = snackbars,
        harpoon = harpoon,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun TaskDetailBody(
    state: TaskDetailUiState,
    zone: ZoneId,
    today: LocalDate,
    moveOptions: List<MoveProject>,
    callbacks: TaskDetailCallbacks,
    snackbars: SnackbarHostState,
    harpoon: @Composable () -> Unit,
    modifier: Modifier = Modifier,
) {
    var field by remember { mutableStateOf<TaskField?>(null) }
    val scroll = rememberScrollState()
    // The title moves into the bar once the one on the page has gone: a detail
    // screen with no heading anywhere says nothing about what is being edited.
    // The distance is in dp, since the same number of pixels is a different
    // amount of scrolling on every phone.
    val handover = with(LocalDensity.current) { TITLE_HANDOVER_DP.dp.toPx() }
    val titleInBar by remember(handover) { derivedStateOf { scroll.value > handover } }

    Scaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            TaskDetailBar(
                task = state.task,
                title = if (titleInBar) state.task?.title.orEmpty() else "",
                onBack = callbacks.onBack,
                onTogglePin = callbacks.onTogglePin,
                onOpenActions = { field = TaskField.ACTIONS },
                harpoon = harpoon,
            )
        },
        snackbarHost = { SnackbarHost(hostState = snackbars) },
    ) { insets ->
        PullToRefreshBox(
            isRefreshing = state.refreshing,
            onRefresh = callbacks.onRefresh,
            modifier = Modifier.fillMaxSize().padding(insets),
        ) {
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
                return@PullToRefreshBox
            }

            Column(
                modifier =
                    Modifier
                        .fillMaxSize()
                        .verticalScroll(scroll)
                        .padding(bottom = 28.dp),
            ) {
                Hero(
                    state = state,
                    callbacks = callbacks,
                    onOpenMove = { field = TaskField.MOVE },
                )
                if (state.blockers.isNotEmpty()) BlockedNotice(state.blockers, callbacks.onOpenTask)

                SectionHeading(stringResource(R.string.native_task_sectionWhen))
                WhenCard(task = task, zone = zone, today = today, onOpen = { field = it })

                SectionHeading(stringResource(R.string.native_task_sectionPlanning))
                DetailCard {
                    PrioritySelector(task.priority, callbacks.onPriority, locked = state.priorityLocked)
                    HorizontalDivider(modifier = Modifier.padding(horizontal = 16.dp))
                    DetailRow(
                        label = stringResource(R.string.native_task_plan),
                        value = planLabel(task.planState),
                        leading = Icons.Outlined.Splitscreen,
                        valueTint = MaterialTheme.colorScheme.onSurface.takeIf { task.planState != PlanState.NONE },
                        onClick = { field = TaskField.PLAN },
                    )
                }

                LabelsSection(
                    names = task.labels.map { it.name },
                    knownLabels = state.knownLabels.isNotEmpty(),
                    onOpen = { field = TaskField.LABELS },
                )

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

                TaskDetailRelations(
                    relations = state.relations,
                    candidates = state.relationCandidates,
                    onSearch = callbacks.onSearchRelations,
                    onAdd = callbacks.onAddRelation,
                    onRemove = callbacks.onRemoveRelation,
                    onOpen = callbacks.onOpenTask,
                )

                SectionHeading(stringResource(R.string.native_task_sectionProperties))
                DetailCard {
                    DetailSwitchRow(
                        label = stringResource(R.string.task_complexMarker),
                        leading = Icons.Outlined.Layers,
                        checked = task.isComplex,
                        onCheckedChange = callbacks.onComplex,
                    )
                    DetailSwitchRow(
                        label = stringResource(R.string.common_privateMarker),
                        leading = Icons.Outlined.Lock,
                        checked = task.isPrivate,
                        onCheckedChange = callbacks.onPrivate,
                    )
                }

                Box(modifier = Modifier.padding(top = 20.dp)) {
                    RecordOfTheTask(task = task, zone = zone)
                }
            }
        }
    }

    TaskFieldSheets(
        field = field,
        state = state,
        zone = zone,
        today = today,
        moveOptions = moveOptions,
        callbacks = callbacks,
        onField = { field = it },
    )
}

/**
 * The sheets every field on the screen opens, in one place.
 *
 * They are hosted by the screen rather than by the rows that raise them so that
 * exactly one can be open at a time — the rows are a list, and two sheets over
 * one another is not a state the user can get out of by pressing back once.
 */
@Composable
private fun TaskFieldSheets(
    field: TaskField?,
    state: TaskDetailUiState,
    zone: ZoneId,
    today: LocalDate,
    moveOptions: List<MoveProject>,
    callbacks: TaskDetailCallbacks,
    onField: (TaskField?) -> Unit,
) {
    val task = state.task ?: return
    val onClose = { onField(null) }
    when (field) {
        null -> Unit

        TaskField.DATE ->
            TaskDateSheet(
                title = stringResource(R.string.native_task_dateAndTime),
                date = task.dueDate(zone),
                time = task.dueTime(zone),
                today = today,
                onPickDate = callbacks.onDueDate,
                onPickTime = callbacks.onDueTime,
                onDismiss = onClose,
            )

        TaskField.DEADLINE ->
            TaskDateSheet(
                title = stringResource(R.string.native_task_deadline),
                date = task.deadlineDate(zone),
                time = task.deadlineTime(zone),
                today = today,
                onPickDate = callbacks.onDeadlineDate,
                onPickTime = callbacks.onDeadlineTime,
                onDismiss = onClose,
            )

        TaskField.DAY_PART -> DayPartSheet(task.dayPart, callbacks.onDayPart, onClose)

        TaskField.PLAN -> PlanSheet(task.planState, callbacks.onPlanState, onClose)

        TaskField.REPEAT ->
            RecurrenceSheet(
                rule = task.recurrenceRule,
                dueAt = task.dueAt,
                from = today.atStartOfDay(zone).toInstant().toEpochMilli(),
                zone = zone,
                onChange = callbacks.onRecurrence,
                onDismiss = onClose,
            )

        TaskField.LABELS ->
            LabelsSheet(
                known = state.knownLabels,
                selected = task.labels.map { it.name },
                onChange = callbacks.onLabels,
                onDismiss = onClose,
            )

        TaskField.ACTIONS ->
            TaskActionsSheet(
                task = task,
                onCancelTask = callbacks.onCancelTask,
                onDuplicate = callbacks.onDuplicate,
                onOpenMove = { onField(TaskField.MOVE) },
                onDelete = callbacks.onDelete,
                onDismiss = onClose,
                // A task with work under it cannot be split: the work would have
                // nowhere to go, and the server refuses it for the same reason.
                canDecompose = state.openSubtasks.isEmpty() && state.doneSubtasks.isEmpty(),
                onDecompose = callbacks.onDecompose,
                onCreateTemplate = callbacks.onCreateTemplate,
            )

        TaskField.MOVE ->
            TaskMoveSheet(
                projects = moveOptions,
                onPick = { destination ->
                    onClose()
                    callbacks.onMove(destination)
                },
                onDismiss = onClose,
            )
    }
}

/**
 * The screen's own chrome: where to go back to, and what can be done here.
 *
 * The screen owns its top bar rather than borrowing the shell's, which carries a
 * menu button and the name of a destination. Neither is what a person looking at
 * one task needs, and a back arrow drawn underneath the shell's bar — which is
 * what this replaced — reads as two navigations stacked on top of each other.
 *
 * Pinning sits here rather than among the task's fields because it is not one: a
 * pin is a place on a shelf with a cap on it, and the write is what refuses a pin
 * over that cap. A switch would promise it always works.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun TaskDetailBar(
    task: Task?,
    title: String,
    onBack: () -> Unit,
    onTogglePin: () -> Unit,
    onOpenActions: () -> Unit,
    harpoon: @Composable () -> Unit,
) {
    TopAppBar(
        title = {
            Text(
                text = title,
                style = MaterialTheme.typography.titleMedium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        },
        navigationIcon = {
            IconButton(onClick = onBack) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                    contentDescription = stringResource(R.string.common_back),
                )
            }
        },
        actions = {
            if (task != null) {
                IconButton(onClick = onTogglePin) {
                    Icon(
                        imageVector = if (task.isPinned) Icons.Filled.PushPin else Icons.Outlined.PushPin,
                        contentDescription =
                            stringResource(
                                if (task.isPinned) R.string.task_actions_unpin else R.string.task_actions_pin,
                            ),
                        tint =
                            if (task.isPinned) {
                                MaterialTheme.colorScheme.primary
                            } else {
                                MaterialTheme.colorScheme.onSurfaceVariant
                            },
                    )
                }
            }
            harpoon()
            if (task != null) {
                IconButton(onClick = onOpenActions) {
                    Icon(
                        imageVector = Icons.Filled.MoreVert,
                        contentDescription = stringResource(R.string.native_task_more),
                    )
                }
            }
        },
        colors = TopAppBarDefaults.topAppBarColors(),
    )
}

/**
 * Where the task lives, what it is called, and what it says.
 *
 * The three read as one block because they are the task itself; everything under
 * them is a field of it. The title is drawn as the heading it is rather than as
 * a labelled text box — a box around a heading says "form", and this screen is
 * not one — and turns into an editor under the cursor when it is tapped.
 */
@Composable
private fun Hero(
    state: TaskDetailUiState,
    callbacks: TaskDetailCallbacks,
    onOpenMove: () -> Unit,
) {
    val task = state.task ?: return
    val completed = task.status == TaskStatus.COMPLETED

    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp)) {
        PlacementCrumb(state.placement, onOpenMove)
        Row(
            modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalAlignment = Alignment.Top,
        ) {
            CompletionControl(
                task = task,
                blocked = state.blocked,
                completed = completed,
                onToggleComplete = callbacks.onToggleComplete,
            )
            TitleField(
                value = task.title,
                strikeThrough = completed,
                onCommit = callbacks.onRename,
                modifier = Modifier.weight(1f).padding(top = 6.dp),
            )
        }
        DescriptionBlock(task.description, callbacks.onDescribe)
    }
}

/**
 * The tick control, and the two things it can be instead of one.
 *
 * A blocked task shows a padlock instead of an empty circle and cannot be
 * ticked, which is the same swap every list row makes: a greyed-out circle still
 * reads as "a circle you may tick later", and the padlock says what is true. The
 * tonal circle around it is what makes it a control on a screen that has no other
 * button at its head.
 */
@Composable
private fun CompletionControl(
    task: Task,
    blocked: Boolean,
    completed: Boolean,
    onToggleComplete: () -> Unit,
) {
    val tint = priorityTint(task.priority)
    FilledTonalIconButton(
        onClick = onToggleComplete,
        enabled = !blocked,
        colors =
            IconButtonDefaults.filledTonalIconButtonColors(
                containerColor =
                    if (blocked) {
                        tint.copy(alpha = BLOCKED_RING_ALPHA)
                    } else {
                        MaterialTheme.colorScheme.surfaceContainerHigh
                    },
                disabledContainerColor = tint.copy(alpha = BLOCKED_RING_ALPHA),
                disabledContentColor = tint,
            ),
    ) {
        when {
            blocked ->
                Icon(
                    imageVector = Icons.Outlined.Lock,
                    contentDescription = stringResource(R.string.task_blockedTooltip),
                    tint = tint,
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
                    tint = tint,
                )
        }
    }
}

/**
 * The title, edited where it stands.
 *
 * It reports what was typed once, when the user is done with it. Reporting on
 * every keystroke would turn one sentence into one queued write per character;
 * reporting a value that did not change would queue a write for merely having
 * looked at the field. So the commit happens when focus leaves or when the
 * keyboard's action is taken, and what it reports is compared with what the task
 * already says before anything is sent.
 *
 * The field wraps rather than scrolling sideways, because a title too long to
 * see is a title nobody can check. Line breaks are folded out on the way through:
 * the wrapping is the screen's, not the title's.
 */
@Composable
private fun TitleField(
    value: String,
    strikeThrough: Boolean,
    onCommit: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    var text by remember(value) { mutableStateOf(value) }
    var hadFocus by remember { mutableStateOf(false) }
    val label = stringResource(R.string.native_task_editTitle)
    val placeholder = stringResource(R.string.page_task_namePlaceholder)
    val style =
        MaterialTheme.typography.headlineSmall.copy(
            color =
                if (strikeThrough) {
                    MaterialTheme.colorScheme.onSurfaceVariant
                } else {
                    MaterialTheme.colorScheme.onSurface
                },
            textDecoration = if (strikeThrough) TextDecoration.LineThrough else null,
        )
    val commit = { onCommit(text.replace('\n', ' ').trim()) }

    BasicTextField(
        value = text,
        onValueChange = { text = it },
        textStyle = style,
        maxLines = TITLE_MAX_LINES,
        cursorBrush = SolidColor(MaterialTheme.colorScheme.primary),
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
        keyboardActions = KeyboardActions(onDone = { commit() }),
        decorationBox = { inner ->
            Box {
                if (text.isEmpty()) {
                    Text(text = placeholder, style = style.copy(color = MaterialTheme.colorScheme.onSurfaceVariant))
                }
                inner()
            }
        },
        modifier =
            modifier
                .semantics { contentDescription = label }
                .onFocusChanged { focus ->
                    if (focus.isFocused) {
                        hadFocus = true
                    } else if (hadFocus) {
                        hadFocus = false
                        commit()
                    }
                },
    )
}

/** Where the task lives, and the way to move it. */
@Composable
private fun PlacementCrumb(
    placement: TaskPlacement,
    onOpenMove: () -> Unit,
) {
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
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .clickable(onClick = onOpenMove)
                .defaultMinSize(minHeight = 40.dp)
                .padding(vertical = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = Icons.Outlined.Folder,
            contentDescription = stringResource(R.string.native_task_location),
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(20.dp),
        )
        Text(
            text = value,
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        Icon(
            imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(18.dp),
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
        shape = RoundedCornerShape(20.dp),
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
    ) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Icon(
                    imageVector = Icons.Outlined.Lock,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onErrorContainer,
                    modifier = Modifier.size(16.dp),
                )
                Text(
                    text = stringResource(R.string.page_task_relation_blockedBy),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onErrorContainer,
                )
            }
            for (blocker in blockers) {
                Text(
                    text = blocker.title,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onErrorContainer,
                    modifier =
                        Modifier
                            .fillMaxWidth()
                            .clickable { onOpenTask(blocker.taskLocalId) }
                            .padding(vertical = 4.dp),
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
    val focusRequester = remember { FocusRequester() }
    var text by remember(description) { mutableStateOf(description) }
    var hadFocus by remember { mutableStateOf(false) }

    if (editing) {
        LaunchedEffect(Unit) { focusRequester.requestFocus() }
        OutlinedTextField(
            value = text,
            onValueChange = { text = it },
            label = { Text(stringResource(R.string.page_task_descriptionPlaceholder)) },
            textStyle = MaterialTheme.typography.bodyMedium,
            modifier =
                Modifier
                    .fillMaxWidth()
                    .padding(vertical = 8.dp)
                    .focusRequester(focusRequester)
                    .onFocusChanged { focus ->
                        if (focus.isFocused) {
                            hadFocus = true
                        } else if (hadFocus) {
                            hadFocus = false
                            editing = false
                            onCommit(text)
                        }
                    },
        )
        return
    }
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .clickable { editing = true }
                .defaultMinSize(minHeight = 44.dp)
                .padding(start = 52.dp, top = 4.dp, bottom = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (description.isEmpty()) {
            Icon(
                imageVector = Icons.AutoMirrored.Outlined.Notes,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(20.dp),
            )
            Text(
                text = stringResource(R.string.native_task_addDescription),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            return@Row
        }
        if (Markdown.hasContent(description)) {
            MarkdownText(description, modifier = Modifier.weight(1f))
            return@Row
        }
        Text(
            text = description,
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(1f),
        )
    }
}

/**
 * When the task is meant to happen, in one card.
 *
 * The four rows are one question asked four ways — which day, which part of it,
 * by when at the latest, and how often it comes back — so they sit together, and
 * each opens the sheet that answers it.
 */
@Composable
private fun WhenCard(
    task: Task,
    zone: ZoneId,
    today: LocalDate,
    onOpen: (TaskField) -> Unit,
) {
    val due = task.dueDate(zone)
    val dueTime = timeLabel(task.dueTime(zone))
    val overdue = due != null && due < today && task.status == TaskStatus.OPEN
    val deadline = task.deadlineDate(zone)
    val rule = task.recurrenceRule.orEmpty()
    val wording = recurrenceWording(rule)
    // A task that comes back is marked in the same green every list marks it in.
    val repeating = rule.isNotBlank() && wording != RecurrenceWording.Unreadable
    val repeatTint = TurboistTheme.accents.recurring

    DetailCard {
        DetailRow(
            label = stringResource(R.string.page_task_date),
            value = listOfNotNull(dateLabel(due, today), dueTime).joinToString(", "),
            leading = Icons.Outlined.CalendarMonth,
            valueTint =
                when {
                    overdue -> MaterialTheme.colorScheme.error
                    due != null -> MaterialTheme.colorScheme.onSurface
                    else -> null
                },
            onClick = { onOpen(TaskField.DATE) },
        )
        DetailRow(
            label = stringResource(R.string.page_task_dayPart),
            value = stringResource(dayPartLabel(task.dayPart)),
            leading = Icons.Outlined.WbSunny,
            valueTint = MaterialTheme.colorScheme.onSurface.takeIf { task.dayPart != DayPart.NONE },
            onClick = { onOpen(TaskField.DAY_PART) },
        )
        DetailRow(
            label = stringResource(R.string.native_task_deadline),
            value = dateLabel(deadline, today),
            leading = Icons.Outlined.Flag,
            valueTint = MaterialTheme.colorScheme.onSurface.takeIf { deadline != null },
            onClick = { onOpen(TaskField.DEADLINE) },
        )
        DetailRow(
            label = stringResource(R.string.page_task_repeat),
            value = recurrenceLabel(wording),
            leading = Icons.Outlined.Repeat,
            leadingTint = repeatTint.takeIf { repeating },
            supporting =
                nextOccurrenceLabel(
                    rule = rule,
                    dueAt = task.dueAt,
                    from = today.atStartOfDay(zone).toInstant().toEpochMilli(),
                    zone = zone,
                ).takeIf { repeating },
            valueTint = repeatTint.takeIf { repeating },
            onClick = { onOpen(TaskField.REPEAT) },
        )
    }
}

/**
 * The labels this task carries, and the way to change them.
 *
 * Only the task's own are here. The workspace's whole set used to be laid out
 * underneath, which made a page of a phone screen out of a field whose answer is
 * usually two words, and left the two that were actually on the task
 * indistinguishable from the eighteen that were not.
 */
@Composable
private fun LabelsSection(
    names: List<String>,
    knownLabels: Boolean,
    onOpen: () -> Unit,
) {
    if (names.isEmpty() && !knownLabels) return
    SectionHeading(text = stringResource(R.string.page_task_labels)) {
        TextButton(onClick = onOpen) { Text(stringResource(R.string.native_task_change)) }
    }
    if (names.isEmpty()) {
        AssistChip(
            onClick = onOpen,
            label = { Text(stringResource(R.string.common_add)) },
            modifier = Modifier.padding(horizontal = 20.dp),
        )
        return
    }
    FlowRow(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        for (name in names) {
            InputChip(
                selected = true,
                onClick = onOpen,
                label = { Text(name) },
                modifier = Modifier.semantics { contentDescription = name },
            )
        }
    }
}

/**
 * What the task has been through: how often it has been put off, and when it was
 * written down, last changed and finished.
 *
 * All of it is read-only — a postpone count that could be edited would stop being
 * a record of anything, and the timestamps belong to the server — and all of it
 * is folded away, because it is the part of the screen a person reads once a
 * month and the fields above it are the part they came for. The summary line is
 * what makes folding it honest: the two facts anyone actually wants from the
 * block are on the row that opens it.
 */
@Composable
private fun RecordOfTheTask(
    task: Task,
    zone: ZoneId,
) {
    var open by remember { mutableStateOf(false) }
    val status = statusLabel(task.status)

    DetailCard {
        DetailRow(
            label = stringResource(R.string.native_task_details),
            supporting = stringResource(R.string.native_task_detailsSummary, status, momentLabel(task.updatedAt, zone)),
            value = "",
            leading = Icons.Outlined.Info,
            trailing = if (open) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore,
            onClick = { open = !open },
        )
        AnimatedVisibility(visible = open) {
            Column {
                HorizontalDivider(modifier = Modifier.padding(horizontal = 16.dp))
                DetailRow(
                    label = stringResource(R.string.native_task_postponed),
                    value = task.postponeCount.toString(),
                )
                DetailRow(label = stringResource(R.string.native_task_status), value = status)
                DetailRow(
                    label = stringResource(R.string.native_task_created),
                    value = momentLabel(task.createdAt, zone),
                )
                DetailRow(
                    label = stringResource(R.string.native_task_updated),
                    value = momentLabel(task.updatedAt, zone),
                )
                if (task.completedAt != null) {
                    DetailRow(
                        label = stringResource(R.string.native_task_completedAt),
                        value = momentLabel(task.completedAt, zone),
                    )
                }
            }
        }
    }
}

/**
 * The screen as the app wires it: a view model behind it, and the things only the
 * navigation graph can supply — how to open another task, how to leave, and the
 * jump-pair control, which navigates.
 */
@Composable
fun TaskDetailScreen(
    address: TaskAddress,
    onOpenTask: (Long) -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    harpoon: @Composable () -> Unit = {},
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
        harpoon = harpoon,
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

/** How far the page scrolls before the title moves into the bar. */
private const val TITLE_HANDOVER_DP = 72

/** How many lines of a title are shown before it is cut off. */
private const val TITLE_MAX_LINES = 4

/** How much of the priority's colour the tick control keeps while it is barred. */
private const val BLOCKED_RING_ALPHA = 0.16f
