package ru.tinyops.turboist.core.network

import kotlinx.coroutines.runBlocking
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.dto.SyncEntity
import ru.tinyops.turboist.core.network.dto.SyncOp
import ru.tinyops.turboist.core.network.dto.TaskDto
import ru.tinyops.turboist.core.network.mapping.toTask
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The delta feed and the seed a replica starts from.
 *
 * Two properties matter here and neither is obvious from the types: a change this
 * build does not understand must not poison the page it arrived in, and a delete
 * carries nothing but an id — it is the only record that the row ever existed.
 */
class SyncPayloadTest {
    @Test
    fun `a page of changes decodes with its cursor, its payloads and its tombstones`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                jsonResponse(
                    200,
                    """
                    {
                      "epoch": 3,
                      "cursor": 18234,
                      "hasMore": true,
                      "changes": [
                        {"entity":"task","op":"upsert","seq":18201,"id":42,
                         "data":{"id":42,"title":"write it down","status":"open","priority":"high",
                                 "planState":"week","blockedByCount":1,"relationCount":2,
                                 "createdAt":"2024-03-09T12:34:56.789Z","updatedAt":"2024-03-09T12:34:56.789Z"}},
                        {"entity":"label","op":"delete","seq":18209,"id":14},
                        {"entity":"quantum_widget","op":"upsert","seq":18211,"id":7,"data":{"id":7}}
                      ]
                    }
                    """.trimIndent(),
                ),
            )

            val page = runBlocking { fixture.network.sync.changes(since = 18_000, epoch = 3, limit = 500) }

            assertEquals(3L, page.epoch)
            assertEquals(18_234L, page.cursor)
            assertTrue(page.hasMore)
            assertEquals(3, page.changes.size)

            val upsert = page.changes[0]
            assertEquals(SyncEntity.TASK, upsert.entityKind)
            assertEquals(SyncOp.UPSERT, upsert.operation)
            assertNotNull(upsert.data)

            val tombstone = page.changes[1]
            assertEquals(SyncEntity.LABEL, tombstone.entityKind)
            assertEquals(SyncOp.DELETE, tombstone.operation)
            assertEquals(14L, tombstone.id)
            assertNull(tombstone.data)

            // A table added by a newer server survives decoding as the unknown
            // sentinel, so the applier can skip one row instead of failing the page.
            assertEquals(SyncEntity.UNKNOWN, page.changes[2].entityKind)

            val task = TurboistJson.decodeFromJsonElement(TaskDto.serializer(), upsert.data!!).toTask()
            assertEquals(42L, task.serverId)
            assertEquals(Priority.HIGH, task.priority)
            assertEquals(TaskStatus.OPEN, task.status)
            assertEquals(PlanState.WEEK, task.planState)
            assertTrue(task.isBlocked)
            assertEquals(2, task.relationSummary.total)
            assertEquals(WireTime.parse("2024-03-09T12:34:56.789Z"), task.createdAt)
        }
    }

    @Test
    fun `the query names the cursor, the history it belongs to and the page size`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, """{"epoch":1,"cursor":5,"hasMore":false,"changes":[]}"""))

            runBlocking { fixture.network.sync.changes(since = 5, epoch = 1, limit = 200) }

            val url = assertNotNull(fixture.server.takeRequest().url)
            assertEquals("5", url.queryParameter("since"))
            assertEquals("1", url.queryParameter("epoch"))
            assertEquals("200", url.queryParameter("limit"))
        }
    }

    @Test
    fun `an omitted epoch and page size are simply absent from the query`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, """{"epoch":1,"cursor":5,"hasMore":false,"changes":[]}"""))

            runBlocking { fixture.network.sync.changes(since = 0) }

            val url = assertNotNull(fixture.server.takeRequest().url)
            assertEquals("0", url.queryParameter("since"))
            assertNull(url.queryParameter("epoch"))
            assertNull(url.queryParameter("limit"))
        }
    }

    @Test
    fun `the seed carries every collection, the position to continue from and the completed window`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                jsonResponse(
                    200,
                    """
                    {
                      "epoch": 2,
                      "cursor": 900,
                      "completedSince": "2024-01-01T00:00:00.000Z",
                      "tasks": [{"id":1,"title":"one","status":"open"}],
                      "projects": [{"id":5,"contextId":9,"title":"work","projectType":"software","status":"open"}],
                      "sections": [{"id":3,"projectId":5,"title":"doing","position":1}],
                      "contexts": [{"id":9,"name":"home"}],
                      "labels": [{"id":14,"name":"bug","isPrivate":true}],
                      "taskRelations": [{"id":6,"sourceTaskId":1,"targetTaskId":2,"type":"blocks"}],
                      "taskTemplates": [{"id":8,"name":"weekly review","subtasks":[{"id":81,"title":"read notes"}]}],
                      "userSettings": {"locale":"en","maxPinnedTasks":25,"bannerDayPart":"morning"},
                      "appSettings": {"autoLabels":[{"mask":"bug","labelIds":[14],"ignoreCase":true}]},
                      "userState": {"activeContextId": 9}
                    }
                    """.trimIndent(),
                ),
            )

            val snapshot = runBlocking { fixture.network.sync.snapshot() }

            assertEquals(2L, snapshot.epoch)
            assertEquals(900L, snapshot.cursor)
            assertEquals("2024-01-01T00:00:00.000Z", snapshot.completedSince)
            assertEquals(1, snapshot.tasks.size)
            assertEquals(5L, snapshot.projects.single().id)
            assertEquals(3L, snapshot.sections.single().id)
            assertEquals("home", snapshot.contexts.single().name)
            assertTrue(snapshot.labels.single().isPrivate)
            assertEquals(RelationType.BLOCKS.wire, snapshot.taskRelations.single().type)
            assertEquals("read notes", snapshot.taskTemplates.single().subtasks.single().title)
            assertEquals("en", snapshot.userSettings.locale)
            assertEquals(1, snapshot.appSettings.autoLabels.size)
            assertEquals(9L, snapshot.userState["activeContextId"].toString().toLong())
        }
    }
}
