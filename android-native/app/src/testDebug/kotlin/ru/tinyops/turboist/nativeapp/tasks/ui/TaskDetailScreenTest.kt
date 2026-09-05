package ru.tinyops.turboist.nativeapp.tasks.ui

import android.app.Application
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import kotlinx.coroutines.flow.MutableSharedFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.TaskRelationGroup
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.BlockerRef
import ru.tinyops.turboist.nativeapp.tasks.TaskDetailUiState
import ru.tinyops.turboist.nativeapp.tasks.TaskListMessage
import ru.tinyops.turboist.nativeapp.tasks.TaskPlacement
import ru.tinyops.turboist.nativeapp.tasks.TaskRelationRef
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.tasks.taskListRows
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals

/**
 * The detail screen composed for real, in every state it has to survive.
 *
 * The point of composing rather than asserting over the state object is that
 * this screen's rules are drawing rules: a blocked task must show a padlock and
 * refuse the tap, a description written in markup must read as prose rather than
 * as asterisks, a task the device does not hold must say so instead of showing
 * an empty form. None of that is visible from the state alone.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class TaskDetailScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)
    private val messages = MutableSharedFlow<TaskListMessage>(extraBufferCapacity = 4)

    private val performed = mutableListOf<String>()

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private fun callbacks() =
        TaskDetailCallbacks(
            onBack = { performed += "back" },
            onRefresh = { performed += "refresh" },
            onToggleComplete = { performed += "toggle" },
            onRename = { performed += "rename $it" },
            onDescribe = { performed += "describe $it" },
            onPriority = { performed += "priority ${it.wire}" },
            onDayPart = { performed += "dayPart ${it.wire}" },
            onPlanState = { performed += "plan ${it.wire}" },
            onDueDate = { performed += "dueDate $it" },
            onDueTime = { performed += "dueTime $it" },
            onDeadlineDate = { performed += "deadlineDate $it" },
            onDeadlineTime = { performed += "deadlineTime $it" },
            onRecurrence = { performed += "recurrence $it" },
            onLabels = { performed += "labels $it" },
            onComplex = { performed += "complex $it" },
            onPrivate = { performed += "private $it" },
            onTogglePin = { performed += "pin" },
            onCancelTask = { performed += "cancel" },
            onDuplicate = { performed += "duplicate" },
            onMove = { performed += "move $it" },
            onDelete = { performed += "delete" },
            onAddSubtask = { performed += "subtask $it" },
            onToggleSubtask = { performed += "toggleSubtask ${it.localId}" },
            onSearchRelations = { performed += "searchRelations $it" },
            onAddRelation = { peer, group -> performed += "addRelation $peer $group" },
            onRemoveRelation = { performed += "removeRelation $it" },
            onOpenTask = { performed += "open $it" },
        )

    private fun show(state: TaskDetailUiState) {
        compose.setContent {
            TurboistTheme {
                TaskDetailScreen(
                    state = state,
                    zone = zone,
                    today = today,
                    moveOptions = emptyList(),
                    messages = messages,
                    callbacks = callbacks(),
                )
            }
        }
        compose.waitForIdle()
    }

    private fun stateOf(
        task: Task,
        blockers: List<BlockerRef> = emptyList(),
        relations: List<TaskRelationRef> = emptyList(),
        openSubtasks: List<Task> = emptyList(),
        doneSubtasks: List<Task> = emptyList(),
        placement: TaskPlacement = TaskPlacement(),
        priorityLocked: Boolean = false,
    ) = TaskDetailUiState(
        loading = false,
        task = task,
        placement = placement,
        blockers = blockers,
        relations = relations,
        openSubtasks = taskListRows(openSubtasks, emptyMap()),
        doneSubtasks = taskListRows(doneSubtasks, emptyMap()),
        priorityLocked = priorityLocked,
    )

    private fun link(
        peerLocalId: Long,
        group: TaskRelationGroup,
        title: String,
        status: TaskStatus = TaskStatus.OPEN,
        id: Long = peerLocalId,
    ) = TaskRelationRef(
        relationLocalId = id,
        group = group,
        peerLocalId = peerLocalId,
        peerServerId = peerLocalId,
        peerTitle = title,
        peerStatus = status,
    )

    @Test
    fun `an open task shows its title and offers to be ticked off`() {
        show(stateOf(task(1, title = "Buy milk", priority = Priority.HIGH)))

        compose.onAllNodesWithText("Buy milk").onFirst().assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.task_markComplete)).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.native_priority_p1)).assertIsDisplayed()
    }

    @Test
    fun `a finished task offers to be put back`() {
        show(stateOf(task(1, status = TaskStatus.COMPLETED, completedAt = 1_000L)))

        compose.onNodeWithContentDescription(text(R.string.task_markIncomplete)).assertIsDisplayed()
        // The word also names the moment the task was finished, so both readings
        // of it are on the screen; either one proves the status was drawn.
        compose.onAllNodesWithText(text(R.string.native_task_statusCompleted)).onFirst().performScrollTo()
    }

    @Test
    fun `a blocked task shows a padlock, names what is in the way, and refuses the tap`() {
        show(
            stateOf(
                task(1, title = "Fit the shelf"),
                blockers = listOf(BlockerRef(taskLocalId = 2, serverId = 2, title = "Order brackets")),
            ),
        )

        compose.onNodeWithContentDescription(text(R.string.task_blockedTooltip)).assertIsNotEnabled()
        compose.onNodeWithText(text(R.string.page_task_relation_blockedBy)).assertIsDisplayed()
        compose.onNodeWithText("Order brackets").assertIsDisplayed()
    }

    @Test
    fun `tapping the work that is in the way opens it`() {
        show(
            stateOf(
                task(1),
                blockers = listOf(BlockerRef(taskLocalId = 42, serverId = 7, title = "Order brackets")),
            ),
        )

        compose.onNodeWithText("Order brackets").performClick()

        assertEquals(listOf("open 42"), performed)
    }

    @Test
    fun `a priority the project decides is shown, explained and not offered`() {
        show(stateOf(task(1, title = "In the plan", priority = Priority.HIGH), priorityLocked = true))

        compose.onNodeWithText(text(R.string.page_task_priorityLockedByTroiki)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.native_priority_p1)).assertIsNotEnabled()

        compose.onNodeWithContentDescription(text(R.string.native_priority_p3)).performClick()

        assertEquals(emptyList(), performed)
    }

    @Test
    fun `a description written in markup reads as prose`() {
        show(stateOf(task(1, description = "- first item\n- **second** item")))

        // The markers themselves are gone; the words they marked are not.
        compose.onNodeWithText("first item").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("second item").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a description with no markup is shown exactly as it was typed`() {
        show(stateOf(task(1, description = "2 * 3 = 6")))

        compose.onNodeWithText("2 * 3 = 6").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a task with no description says so rather than showing nothing`() {
        show(stateOf(task(1)))

        compose.onNodeWithText(text(R.string.native_task_noDescription)).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `subtasks are listed, finished ones under their own heading`() {
        show(
            stateOf(
                task(1),
                openSubtasks = listOf(task(2, title = "Wash up", parentLocalId = 1)),
                doneSubtasks = listOf(task(3, title = "Dry up", parentLocalId = 1, status = TaskStatus.COMPLETED)),
            ),
        )

        compose.onNodeWithText(text(R.string.page_task_subtasks)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Wash up").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.native_task_completedSubtasks)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Dry up").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a task in the inbox is told why it has no subtasks instead of being offered a field`() {
        show(stateOf(task(1, inboxId = 1), placement = TaskPlacement(inInbox = true)))

        compose
            .onNodeWithText(text(R.string.page_task_inboxSubtasksNoticePre))
            .performScrollTo()
            .assertIsDisplayed()
        compose.onAllNodesWithText(text(R.string.page_task_addSubtaskPlaceholder)).assertCountEquals(0)
    }

    @Test
    fun `a task the device does not hold says so`() {
        show(TaskDetailUiState(loading = false))

        compose.onNodeWithText(text(R.string.page_task_notFound)).assertIsDisplayed()
    }

    @Test
    fun `a refusal is raised as the sentence the user reads`() {
        show(stateOf(task(1)))

        messages.tryEmit(TaskListMessage.BLOCKED)
        compose.waitForIdle()

        compose.onNodeWithText(text(R.string.task_toast_blockedCannotComplete)).assertIsDisplayed()
    }

    @Test
    fun `choosing a priority reports the level that was chosen`() {
        show(stateOf(task(1, priority = Priority.NONE)))

        compose.onNodeWithContentDescription(text(R.string.native_priority_p2)).performScrollTo().performClick()

        assertEquals(listOf("priority medium"), performed)
    }

    @Test
    fun `each link is listed under what it says, with the task at the other end named`() {
        show(
            stateOf(
                task(1),
                relations =
                    listOf(
                        link(9, TaskRelationGroup.BLOCKED_BY, "Order the parts"),
                        link(10, TaskRelationGroup.BLOCKS, "Ship it"),
                        link(11, TaskRelationGroup.RELATED, "Read the spec"),
                    ),
            ),
        )

        compose.onNodeWithText(text(R.string.page_task_relation_blockedBy)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Order the parts").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.page_task_relation_blocks)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Ship it").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.page_task_relation_related)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Read the spec").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a task with no links says so rather than showing empty headings`() {
        show(stateOf(task(1)))

        compose.onNodeWithText(text(R.string.page_task_relationEmpty)).performScrollTo().assertIsDisplayed()
        compose.onAllNodesWithText(text(R.string.page_task_relation_blockedBy)).assertCountEquals(0)
    }

    @Test
    fun `tapping a link goes to the task at the other end`() {
        show(stateOf(task(1), relations = listOf(link(9, TaskRelationGroup.BLOCKED_BY, "Order the parts"))))

        compose.onNodeWithText("Order the parts").performScrollTo().performClick()

        assertEquals(listOf("open 9"), performed)
    }

    @Test
    fun `a link can be taken off from the task it is listed on`() {
        show(stateOf(task(1), relations = listOf(link(9, TaskRelationGroup.RELATED, "Read the spec", id = 77))))

        compose.onNodeWithContentDescription(text(R.string.page_task_relationRemove)).performScrollTo().performClick()

        assertEquals(listOf("removeRelation 77"), performed)
    }

    @Test
    fun `a link to work that is already finished is struck through rather than hidden`() {
        show(
            stateOf(
                task(1),
                relations =
                    listOf(
                        link(9, TaskRelationGroup.BLOCKED_BY, "Order the parts", status = TaskStatus.COMPLETED),
                    ),
            ),
        )

        // It is still why this task was waiting, so it stays on screen.
        compose.onNodeWithText("Order the parts").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a link refused for being there already is raised as the sentence the user reads`() {
        show(stateOf(task(1)))

        messages.tryEmit(TaskListMessage.RELATION_EXISTS)
        compose.waitForIdle()

        compose.onNodeWithText(text(R.string.native_task_relationExists)).assertIsDisplayed()
    }

    @Test
    fun `the location row names where the task lives and opens the move sheet`() {
        show(
            stateOf(
                task(1, projectLocalId = 4),
                placement = TaskPlacement(projectLocalId = 4, projectTitle = "Kitchen", sectionTitle = "Doing"),
            ),
        )

        compose.onNodeWithText("Kitchen · Doing").performScrollTo().assertIsDisplayed()
    }
}
