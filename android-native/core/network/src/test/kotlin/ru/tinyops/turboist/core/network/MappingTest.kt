package ru.tinyops.turboist.core.network

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.HarpoonKind
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.RelationDirection
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.dto.AppSettingsDto
import ru.tinyops.turboist.core.network.dto.AutoLabelRuleDto
import ru.tinyops.turboist.core.network.dto.HarpoonDto
import ru.tinyops.turboist.core.network.dto.HarpoonSlotDto
import ru.tinyops.turboist.core.network.dto.LabelDto
import ru.tinyops.turboist.core.network.dto.ProjectDto
import ru.tinyops.turboist.core.network.dto.TaskDto
import ru.tinyops.turboist.core.network.dto.TaskRelationDto
import ru.tinyops.turboist.core.network.dto.TaskTemplateDto
import ru.tinyops.turboist.core.network.dto.TaskTemplateSubtaskDto
import ru.tinyops.turboist.core.network.dto.UserSettingsDto
import ru.tinyops.turboist.core.network.mapping.ReplicaIds
import ru.tinyops.turboist.core.network.mapping.toAppSettings
import ru.tinyops.turboist.core.network.mapping.toHarpoon
import ru.tinyops.turboist.core.network.mapping.toProject
import ru.tinyops.turboist.core.network.mapping.toRelation
import ru.tinyops.turboist.core.network.mapping.toTask
import ru.tinyops.turboist.core.network.mapping.toTemplate
import ru.tinyops.turboist.core.network.mapping.toUserSettings
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** A replica whose rows have identities of their own, unrelated to the server's. */
private class FakeIds(private val mapping: Map<Long, Long>) : ReplicaIds {
    override fun task(serverId: Long?): Long? = serverId?.let(mapping::get)

    override fun project(serverId: Long?): Long? = serverId?.let(mapping::get)

    override fun section(serverId: Long?): Long? = serverId?.let(mapping::get)

    override fun context(serverId: Long?): Long? = serverId?.let(mapping::get)

    override fun label(serverId: Long?): Long? = serverId?.let(mapping::get)

    override fun template(serverId: Long?): Long? = serverId?.let(mapping::get)
}

class MappingTest {
    @Test
    fun `references become local ids while the server id is kept alongside`() {
        val ids = FakeIds(mapOf(42L to 100L, 5L to 200L, 3L to 300L, 9L to 400L, 1L to 500L))
        val dto =
            TaskDto(
                id = 42,
                title = "write it down",
                projectId = 5,
                sectionId = 3,
                contextId = 9,
                parentId = 1,
                priority = "high",
                status = "open",
                dayPart = "morning",
                createdAt = "2024-03-09T12:34:56.789Z",
                updatedAt = "2024-03-09T12:34:56.789Z",
            )

        val task = dto.toTask(ids)

        assertEquals(100L, task.localId)
        assertEquals(42L, task.serverId)
        assertEquals(200L, task.projectLocalId)
        assertEquals(300L, task.sectionLocalId)
        assertEquals(400L, task.contextLocalId)
        assertEquals(500L, task.parentLocalId)
        assertEquals(Priority.HIGH, task.priority)
        assertEquals(DayPart.MORNING, task.dayPart)
        assertEquals(WireTime.parse("2024-03-09T12:34:56.789Z"), task.updatedAt)
    }

    @Test
    fun `a row the replica has not seen yet maps to no local id rather than to a wrong one`() {
        val task = TaskDto(id = 42, projectId = 5).toTask(FakeIds(emptyMap()))

        assertEquals(NO_LOCAL_ID, task.localId)
        assertNull(task.projectLocalId)
    }

    @Test
    fun `a value this build has never heard of degrades instead of failing the row`() {
        val task = TaskDto(id = 1, title = "still readable", priority = "apocalyptic", status = "quantum").toTask()

        assertEquals(Priority.UNKNOWN, task.priority)
        assertEquals("still readable", task.title)
    }

    @Test
    fun `a missing timestamp costs the moment, not the row`() {
        val task = TaskDto(id = 1, title = "no dates", createdAt = null, dueAt = "nonsense").toTask()

        assertEquals(0L, task.createdAt)
        assertNull(task.dueAt)
    }

    @Test
    fun `the relation rollup is what makes a list able to draw a task as blocked`() {
        val task = TaskDto(id = 1, blockedByCount = 2, relationCount = 3).toTask()

        assertTrue(task.isBlocked)
        assertEquals(2, task.relationSummary.blockedByOpen)
        assertEquals(3, task.relationSummary.total)
    }

    @Test
    fun `a relation read from one end recovers both ends of the stored edge`() {
        val incoming =
            TaskRelationDto(
                id = 6,
                type = "blocks",
                direction = "incoming",
                task = TaskDto(id = 7, title = "the blocker"),
            ).toRelation(ownerLocalId = 1)

        // Incoming means the peer blocks the owner, so the peer is the source.
        assertEquals(7L, incoming.sourceTaskLocalId)
        assertEquals(1L, incoming.targetTaskLocalId)
        assertEquals(RelationDirection.INCOMING, incoming.direction)
        assertEquals(RelationType.BLOCKS, incoming.type)

        val outgoing =
            TaskRelationDto(
                id = 7,
                type = "blocks",
                direction = "outgoing",
                task = TaskDto(id = 9, title = "the dependent"),
            ).toRelation(ownerLocalId = 1)

        assertEquals(1L, outgoing.sourceTaskLocalId)
        assertEquals(9L, outgoing.targetTaskLocalId)
    }

    @Test
    fun `a list payload does not wipe relations it was never going to carry`() {
        val task = TaskDto(id = 1, relationCount = 4).toTask()

        assertTrue(task.relations.isEmpty())
        assertEquals(4, task.relationSummary.total)
    }

    @Test
    fun `a project keeps its board type and its capacity bucket`() {
        val project =
            ProjectDto(
                id = 5,
                contextId = 9,
                title = "work",
                projectType = "software",
                troikiCategory = "important",
                labels = listOf(LabelDto(id = 14, name = "bug")),
            ).toProject()

        assertEquals(ProjectType.SOFTWARE, project.type)
        assertEquals(TroikiCategory.IMPORTANT, project.troikiCategory)
        assertEquals("bug", project.labels.single().name)
    }

    @Test
    fun `a project with no capacity bucket keeps none, rather than an unknown one`() {
        assertNull(ProjectDto(id = 5, contextId = 9).toProject().troikiCategory)
    }

    @Test
    fun `template subtasks take their order from the list and their lifetime from the template`() {
        val template =
            TaskTemplateDto(
                id = 8,
                name = "weekly review",
                createdAt = "2024-03-09T12:34:56.789Z",
                updatedAt = "2024-03-10T12:34:56.789Z",
                subtasks =
                    listOf(
                        TaskTemplateSubtaskDto(id = 81, title = "read notes"),
                        TaskTemplateSubtaskDto(id = 82, title = "plan the week"),
                    ),
            ).toTemplate()

        assertEquals(listOf(0, 1), template.subtasks.map { it.position })
        assertEquals(listOf(81L, 82L), template.subtasks.map { it.serverId })
        assertEquals(template.createdAt, template.subtasks.first().createdAt)
        assertEquals(template.localId, template.subtasks.first().templateLocalId)
    }

    @Test
    fun `an all-day banner is an absent phase, not the none phase`() {
        assertNull(UserSettingsDto(bannerDayPart = "").toUserSettings().bannerDayPart)
        assertEquals(DayPart.MORNING, UserSettingsDto(bannerDayPart = "morning").toUserSettings().bannerDayPart)
    }

    @Test
    fun `a pinning cap written before the setting existed falls back to the default`() {
        val settings = UserSettingsDto(maxPinnedTasks = 0, maxPinnedProjects = 25).toUserSettings()

        assertEquals(10, settings.maxPinnedTasks)
        assertEquals(25, settings.maxPinnedProjects)
    }

    @Test
    fun `the jump pair keeps server ids, because it travels back to the server unchanged`() {
        val harpoon =
            HarpoonDto(
                slots = listOf(HarpoonSlotDto(kind = "task", id = 42, title = "write it down")),
            ).toHarpoon()

        assertEquals(HarpoonKind.TASK, harpoon.single().kind)
        assertEquals(42L, harpoon.single().id)
    }

    @Test
    fun `installation rules map across whole, mask and all`() {
        val settings =
            AppSettingsDto(
                autoLabels = listOf(AutoLabelRuleDto(mask = "bug", labelIds = listOf(14), ignoreCase = true)),
            )
                .toAppSettings()

        assertEquals("bug", settings.autoLabels.single().mask)
        assertEquals(listOf(14L), settings.autoLabels.single().labelIds)
        assertTrue(settings.autoLabels.single().ignoreCase)
    }
}
