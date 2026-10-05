package ru.tinyops.turboist.core.model

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class EntitiesTest {
    private fun task(
        localId: Long = 1L,
        serverId: Long? = null,
        summary: TaskRelationSummary = TaskRelationSummary.NONE,
    ) = Task(
        localId = localId,
        serverId = serverId,
        title = "Write the sync applier",
        relationSummary = summary,
        createdAt = 0L,
        updatedAt = 0L,
    )

    @Test
    fun `a fresh entity has no local id until the replica assigns one`() {
        assertEquals(NO_LOCAL_ID, Task(title = "draft", createdAt = 0L, updatedAt = 0L).localId)
    }

    @Test
    fun `a row is unsynced until the server hands back an id`() {
        assertFalse(task().isSynced)
        assertTrue(task(serverId = 42L).isSynced)
    }

    @Test
    fun `the web URL is built from the server id`() {
        assertEquals("https://example.org/task/42", task(serverId = 42L).url("https://example.org"))
    }

    @Test
    fun `a trailing slash on the base URL does not double up`() {
        assertEquals("https://example.org/task/42", task(serverId = 42L).url("https://example.org/"))
    }

    @Test
    fun `a task that has never reached the server has no URL to share`() {
        assertNull(task().url("https://example.org"))
    }

    @Test
    fun `an open blocker makes a task blocked`() {
        assertFalse(task(summary = TaskRelationSummary.NONE).isBlocked)
        assertTrue(task(summary = TaskRelationSummary(blockedByOpen = 1, total = 3)).isBlocked)
    }

    @Test
    fun `relation direction is read from the endpoint being looked at`() {
        val edge =
            TaskRelation(
                localId = 5L,
                sourceTaskLocalId = 1L,
                targetTaskLocalId = 2L,
                type = RelationType.BLOCKS,
                createdAt = 0L,
            )
        assertEquals(RelationDirection.OUTGOING, edge.directionFor(1L))
        assertEquals(RelationDirection.INCOMING, edge.directionFor(2L))
        assertNull(edge.directionFor(3L))
    }

    @Test
    fun `a stored edge carries no point of view of its own`() {
        val edge =
            TaskRelation(
                localId = 5L,
                sourceTaskLocalId = 1L,
                targetTaskLocalId = 2L,
                type = RelationType.BLOCKS,
                createdAt = 0L,
            )
        assertNull(edge.direction)
    }

    @Test
    fun `a task placed in the inbox references the server's singleton inbox`() {
        val inboxTask = task().copy(inboxId = INBOX_ID)
        assertEquals(1L, inboxTask.inboxId)
    }

    @Test
    fun `a field the payload leaves out decodes to the value the server means by it`() {
        // These defaults are the decoder's contract, not decoration: the API omits
        // a zero-valued attribute, so every field left out of a task payload has to
        // land on the same value the server would have sent. A changed default here
        // silently rewrites tasks as they arrive.
        val fresh = task()
        assertEquals(Priority.NONE, fresh.priority)
        assertEquals(TaskStatus.OPEN, fresh.status)
        assertEquals(DayPart.NONE, fresh.dayPart)
        assertEquals(PlanState.NONE, fresh.planState)
        assertFalse(fresh.dueHasTime)
        assertEquals(0, fresh.postponeCount)
        assertTrue(fresh.labels.isEmpty())
        assertTrue(fresh.relations.isEmpty())
    }

    @Test
    fun `a project with no type reads as generic, the way the server stores it`() {
        // Same contract on the project side: an empty type column means generic, and
        // a project is open until something closes it.
        val project = Project(localId = 1L, contextLocalId = 2L, title = "Turboist", createdAt = 0L, updatedAt = 0L)
        assertEquals(ProjectType.GENERIC, project.type)
        assertEquals(ProjectStatus.OPEN, project.status)
        assertNull(project.troikiCategory)
    }
}
