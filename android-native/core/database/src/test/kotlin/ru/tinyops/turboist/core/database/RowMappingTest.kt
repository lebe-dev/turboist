package ru.tinyops.turboist.core.database

import org.junit.Test
import ru.tinyops.turboist.core.database.entity.TaskRelationRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateSubtaskRow
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.database.entity.toRow
import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectSection
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskRelationSummary
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TaskTemplate
import ru.tinyops.turboist.core.model.TroikiCategory
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The translation between a stored row and the type the app works in.
 *
 * A field lost here is a field the user silently stops seeing, so the test
 * builds rows in which no two fields hold the same value: a mapper that swaps
 * two of them, or forgets one, cannot produce an equal round trip.
 */
class RowMappingTest {
    @Test
    fun `a task survives the round trip with every field intact`() {
        val row =
            TaskRow(
                localId = 11,
                serverId = 12,
                title = "Renew passport",
                description = "book the appointment",
                inboxId = 1,
                contextLocalId = 13,
                projectLocalId = 14,
                sectionLocalId = 15,
                parentLocalId = 16,
                priority = Priority.HIGH,
                status = TaskStatus.CANCELLED,
                dueAt = 21,
                dueHasTime = true,
                deadlineAt = 22,
                deadlineHasTime = true,
                dayPart = DayPart.EVENING,
                planState = PlanState.BACKLOG,
                isPinned = true,
                pinnedAt = 23,
                isPrivate = true,
                isComplex = true,
                completedAt = 24,
                recurrenceRule = "FREQ=WEEKLY",
                sourceTaskLocalId = 17,
                postponeCount = 3,
                troikiCategory = TroikiCategory.MEDIUM,
                createdAt = 25,
                updatedAt = 26,
            )

        assertEquals(row, row.toDomain().toRow())
    }

    @Test
    fun `what a task carries beyond its own row is left for the caller to load`() {
        val task = TaskRow(localId = 1, title = "Anything", createdAt = 2, updatedAt = 3).toDomain()

        // A list needs the rollup, a task page needs the edges; neither belongs
        // to the row, so neither is invented here.
        assertTrue(task.labels.isEmpty())
        assertTrue(task.relations.isEmpty())
        assertEquals(TaskRelationSummary.NONE, task.relationSummary)
    }

    @Test
    fun `the workspace types survive the round trip too`() {
        val context =
            Context(
                localId = 1,
                serverId = 2,
                name = "Work",
                color = "blue",
                isFavourite = true,
                createdAt = 3,
                updatedAt = 4,
            )
        assertEquals(context, context.toRow().toDomain())

        val label =
            Label(
                localId = 5,
                serverId = 6,
                name = "bug",
                color = "red",
                isFavourite = true,
                isPrivate = true,
                createdAt = 7,
                updatedAt = 8,
            )
        assertEquals(label, label.toRow().toDomain())

        val project =
            Project(
                localId = 9,
                serverId = 10,
                contextLocalId = 11,
                title = "Website",
                description = "the public one",
                color = "teal",
                status = ProjectStatus.ARCHIVED,
                type = ProjectType.SOFTWARE,
                isPinned = true,
                pinnedAt = 12,
                isPrivate = true,
                troikiCategory = TroikiCategory.IMPORTANT,
                createdAt = 13,
                updatedAt = 14,
            )
        assertEquals(project, project.toRow().toDomain())

        val section =
            ProjectSection(
                localId = 15,
                serverId = 16,
                projectLocalId = 17,
                title = "Doing",
                position = 2,
                createdAt = 18,
                updatedAt = 19,
            )
        assertEquals(section, section.toRow().toDomain())
    }

    @Test
    fun `an edge keeps both of its ends and its kind`() {
        val row =
            TaskRelationRow(
                localId = 1,
                serverId = 2,
                sourceTaskLocalId = 3,
                targetTaskLocalId = 4,
                type = RelationType.BLOCKS,
                createdAt = 5,
            )

        assertEquals(row, row.toDomain().toRow())
    }

    @Test
    fun `a template and its subtasks survive the round trip`() {
        val template =
            TaskTemplate(
                localId = 1,
                serverId = 2,
                name = "Weekly review",
                description = "the checklist",
                priority = Priority.LOW,
                dayPart = DayPart.MORNING,
                position = 3,
                createdAt = 4,
                updatedAt = 5,
            )
        assertEquals(template, template.toRow().toDomain())

        val subtask =
            TaskTemplateSubtaskRow(
                localId = 6,
                serverId = 7,
                templateLocalId = 1,
                position = 2,
                title = "Empty the inbox",
                description = "every last one",
                priority = Priority.MEDIUM,
                dayPart = DayPart.AFTERNOON,
                createdAt = 8,
                updatedAt = 9,
            )
        assertEquals(subtask, subtask.toDomain().toRow())
    }
}
