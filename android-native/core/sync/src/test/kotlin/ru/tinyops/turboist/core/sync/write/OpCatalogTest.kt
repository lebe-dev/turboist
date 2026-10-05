package ru.tinyops.turboist.core.sync.write

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Test
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.Clearable
import ru.tinyops.turboist.core.network.dto.CreateContextRequest
import ru.tinyops.turboist.core.network.dto.CreateLabelRequest
import ru.tinyops.turboist.core.network.dto.CreateProjectRequest
import ru.tinyops.turboist.core.network.dto.CreateSectionRequest
import ru.tinyops.turboist.core.network.dto.CreateTaskRequest
import ru.tinyops.turboist.core.network.dto.PatchContextRequest
import ru.tinyops.turboist.core.network.dto.PatchLabelRequest
import ru.tinyops.turboist.core.network.dto.PatchSectionRequest
import ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The catalog of writes: one type per mutation, each with a name that survives a
 * process death and a request it becomes.
 */
class OpCatalogTest {
    @Test
    fun `every write in the catalog has an op`() {
        assertEquals(
            OutboxOpKind.entries.toSet(),
            SAMPLES.map { it.kind }.toSet(),
            "an op without a sample here is an op nothing has checked can be stored or sent",
        )
    }

    @Test
    fun `an op is named the same in its own column and inside its payload`() {
        // The name is stored twice on purpose — once where a query can filter on
        // it, once inside the payload so a row lifted out of the database still
        // says what it is. Two spellings of the same fact are two chances to
        // disagree, so they are checked against each other.
        for (op in SAMPLES) {
            val stored = OutboxOpCodec.encode(op)
            val named = Json.parseToJsonElement(stored).jsonObject["op"]?.jsonPrimitive?.content
            assertEquals(op.kind.stored, named, "${op::class.simpleName} is named twice and differently")
        }
    }

    @Test
    fun `a stored op reads back as the op that was stored`() {
        for (op in SAMPLES) {
            assertEquals(op, OutboxOpCodec.decode(OutboxOpCodec.encode(op)), "${op.kind.stored} did not survive")
        }
    }

    @Test
    fun `emptying a field survives being stored`() {
        // The three states a PATCH body has are absent, null and a value, and
        // only two of them survive a careless round-trip. A stored "clear this
        // date" that reads back as "leave it alone" would send a request that
        // quietly does nothing — the worst possible failure for a queue whose
        // whole job is to make a change happen later.
        val op = PatchTaskOp(1, PatchTaskPayload(clearDueAt = true))

        val restored = OutboxOpCodec.decode(OutboxOpCodec.encode(op)) as PatchTaskOp

        assertEquals(Clearable.Clear, restored.patch.toRequest().dueAt)
        assertNull(restored.patch.toRequest().title, "a field nobody touched must not come back set")
    }

    @Test
    fun `a name maps back to its op`() {
        for (kind in OutboxOpKind.entries) {
            assertEquals(kind, OutboxOpKind.fromStored(kind.stored))
        }
        assertNull(OutboxOpKind.fromStored("something.this.build.never.heard.of"))
    }

    @Test
    fun `every op knows the request it becomes`() {
        for (op in SAMPLES) {
            val endpoint = op.endpoint()
            assertTrue(endpoint.path.startsWith("api/v1/"), "${op.kind.stored} has no endpoint")
            // A path with a placeholder must say which row fills it, or nothing
            // could ever build the URL; a path without one must not claim a row,
            // or a reference would be resolved for no reason.
            assertEquals(
                endpoint.path.contains("{id}"),
                endpoint.pathRef != null,
                "${op.kind.stored}: the placeholder and the row it stands for disagree",
            )
            assertEquals(
                endpoint.path.contains("{relationId}"),
                endpoint.secondaryRef != null,
                "${op.kind.stored}: the second placeholder and the row it stands for disagree",
            )
        }
    }

    @Test
    fun `where a task is created decides which endpoint creates it`() {
        val body = CreateTaskRequest(title = "Write it down")
        assertEquals(
            "api/v1/inbox/tasks",
            CreateTaskOp(1, TaskDestination.Inbox, body).endpoint().path,
        )
        assertEquals(
            "api/v1/projects/{id}/tasks",
            CreateTaskOp(1, TaskDestination.InProject(7), body).endpoint().path,
        )
        assertEquals(
            OpRef(ReplicaEntityKind.PROJECT, 7),
            CreateTaskOp(1, TaskDestination.InProject(7), body).endpoint().pathRef,
        )
        assertEquals(
            "api/v1/sections/{id}/tasks",
            CreateTaskOp(1, TaskDestination.InSection(3), body).endpoint().path,
        )
        assertEquals(
            "api/v1/contexts/{id}/tasks",
            CreateTaskOp(1, TaskDestination.InContext(2), body).endpoint().path,
        )
        assertEquals(
            "api/v1/tasks/{id}/subtasks",
            CreateTaskOp(1, TaskDestination.SubtaskOf(9), body).endpoint().path,
        )
    }

    @Test
    fun `each project status transition has its own endpoint`() {
        val paths = ProjectStatusAction.entries.map { ProjectStatusOp(1, it).endpoint().path }
        assertEquals(paths.toSet().size, paths.size, "two transitions would be sent to the same place")
        assertEquals("api/v1/projects/{id}/archive", ProjectStatusOp(1, ProjectStatusAction.ARCHIVE).endpoint().path)
    }

    @Test
    fun `removing a relation names both the task and the edge`() {
        val endpoint = RemoveTaskRelationOp(taskLocalId = 4, relationLocalId = 11).endpoint()
        assertEquals("api/v1/tasks/{id}/relations/{relationId}", endpoint.path)
        assertEquals(OpRef(ReplicaEntityKind.TASK, 4), endpoint.pathRef)
        assertEquals(OpRef(ReplicaEntityKind.TASK_RELATION, 11), assertNotNull(endpoint.secondaryRef))
    }

    private companion object {
        /**
         * One of every op. The list is what proves the catalog is complete, so an
         * op added without a line here fails the first test in this file.
         */
        val SAMPLES: List<OutboxOp> =
            listOf(
                CreateTaskOp(1, TaskDestination.Inbox, CreateTaskRequest(title = "Write it down")),
                PatchTaskOp(1, PatchTaskPayload(title = "Renamed")),
                DeleteTaskOp(1),
                CompleteTaskOp(1, "2024-03-09T12:34:56.789Z"),
                UncompleteTaskOp(1),
                CancelTaskOp(1),
                MoveTaskOp(1, TaskDestination.InProject(2)),
                PlanTaskOp(1, "week"),
                PinTaskOp(1),
                UnpinTaskOp(1),
                DuplicateTaskOp(1),
                DecomposeTaskOp(1, listOf("First half", "Second half"), listOf(1, 5)),
                AddTaskRelationOp(1, 2, 3, "blocks", "outgoing"),
                RemoveTaskRelationOp(1, 3),
                BulkCompleteTasksOp(listOf(1, 2)),
                BulkMoveTasksOp(listOf(1, 2), TaskDestination.InSection(4)),
                BulkTaskPriorityOp(listOf(1, 2), "high"),
                GroupTasksOp(1, listOf(2, 3), TaskDestination.InProject(4), title = "Release"),
                CreateProjectOp(1, 2, CreateProjectRequest(title = "Website")),
                PatchProjectOp(1, title = "Renamed"),
                DeleteProjectOp(1),
                ProjectStatusOp(1, ProjectStatusAction.COMPLETE),
                PinProjectOp(1),
                UnpinProjectOp(1),
                SetProjectTroikiOp(1, "important"),
                CreateSectionOp(1, 2, CreateSectionRequest(title = "Doing")),
                PatchSectionOp(1, PatchSectionRequest(title = "Done")),
                DeleteSectionOp(1),
                ReorderSectionOp(1, 2),
                CreateContextOp(1, CreateContextRequest(name = "Work")),
                PatchContextOp(1, PatchContextRequest(name = "Home")),
                DeleteContextOp(1),
                CreateLabelOp(1, CreateLabelRequest(name = "bug")),
                PatchLabelOp(1, PatchLabelRequest(color = "#ff0000")),
                DeleteLabelOp(1),
                CreateTemplateOp(1, TemplatePayload(name = "Weekly review")),
                ReplaceTemplateOp(1, TemplatePayload(name = "Weekly review")),
                DeleteTemplateOp(1),
                InstantiateTemplateOp(1, 2, rootTaskLocalId = 6, subtaskLocalIds = listOf(7, 8)),
                PatchUserSettingsOp(PatchUserSettingsRequest(locale = "en")),
                PutAutoLabelsOp(listOf(AutoLabelRulePayload("fix", listOf(3)))),
                PutProjectSuggestionsOp(listOf(ProjectSuggestionRulePayload("site", listOf(4)))),
                PatchUserStateOp(activeContextId = 2),
                StartTroikiOp,
                ResetTroikiOp,
                AttachHarpoonOp(HarpoonTarget.TASK, 1),
                DetachHarpoonOp(HarpoonTarget.PROJECT, 1),
            )
    }
}
