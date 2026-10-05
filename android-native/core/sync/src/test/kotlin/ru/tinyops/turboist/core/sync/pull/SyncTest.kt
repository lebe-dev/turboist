package ru.tinyops.turboist.core.sync.pull

import androidx.room.Room
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.runBlocking
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import org.junit.After
import org.junit.Before
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.ServerUrl
import ru.tinyops.turboist.core.network.TurboistNetwork
import ru.tinyops.turboist.core.sync.SyncMutex

/**
 * A real replica on one side and a real server on the other.
 *
 * Nothing between them is stubbed: the database is SQLite with its foreign keys
 * live, and the server is an HTTP server answering on a local port through the
 * app's own client stack. That matters because almost everything the pull
 * applier can get wrong — a reference written before its target, a transaction
 * that should have rolled back, a page decoded from a shape the server does not
 * actually send — is invisible to a test that stands in for either side.
 */
@RunWith(RobolectricTestRunner::class)
abstract class SyncTest {
    protected lateinit var db: TurboistDatabase
    protected lateinit var server: MockWebServer
    protected lateinit var network: TurboistNetwork
    protected lateinit var applier: ReplicaApplier
    protected lateinit var puller: SyncPuller

    @Before
    fun openReplicaAndServer() {
        db =
            Room.inMemoryDatabaseBuilder(RuntimeEnvironment.getApplication(), TurboistDatabase::class.java)
                .build()
        server = MockWebServer()
        server.start()
        network = TurboistNetwork.create(serverUrl = ServerUrl(server.url("/").toString()))
        applier = ReplicaApplier(db) { NOW }
        puller = newPuller()
    }

    @After
    fun closeReplicaAndServer() {
        db.close()
        server.close()
    }

    /**
     * Runs a case that talks to the replica and the server.
     *
     * The block's own last value is dropped, which is the point: a test method
     * has to return nothing, and an assertion that happens to answer with a row
     * must not change that.
     */
    protected fun runReplicaTest(block: suspend CoroutineScope.() -> Unit) {
        runBlocking(block = block)
    }

    /** A second engine over the same replica — what a restarted process gets. */
    protected fun newPuller(): SyncPuller = SyncPuller(network.sync, db, applier, SyncMutex())

    // --- what the server answers ---

    protected fun jsonResponse(
        body: String,
        code: Int = 200,
    ): MockResponse =
        MockResponse.Builder()
            .code(code)
            .setHeader("Content-Type", "application/json")
            .body(body)
            .build()

    protected fun enqueueJson(
        body: String,
        code: Int = 200,
    ) {
        server.enqueue(jsonResponse(body, code))
    }

    /**
     * Answers by looking at the request instead of by the order it arrived in.
     *
     * Worth the extra few lines wherever the point of the case is *what the
     * device asked for* — a catch-up resuming from the wrong position would
     * otherwise be answered by the next queued response and look like a pass.
     */
    protected fun respondBy(handler: (RecordedRequest) -> MockResponse) {
        server.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse = handler(request)
            }
    }

    protected fun enqueueError(
        status: Int,
        code: String,
        details: String? = null,
    ) {
        val detailsPart = if (details == null) "" else ""","details":$details"""
        enqueueJson("""{"error":{"code":"$code","message":"refused"$detailsPart}}""", status)
    }

    /**
     * Runs [block] against an address nothing is listening on, which is what
     * having no network looks like from inside the client.
     *
     * Pointing the client elsewhere rather than breaking the connection is what
     * makes the case honest: the call fails the same way and for the same reason
     * it would on a train, and the server is still there afterwards for the
     * recovery half of the case.
     */
    protected fun <T> withServerUnreachable(block: () -> T): T {
        val reachable = server.url("/").toString()
        network.serverUrl.set(UNREACHABLE_ADDRESS)
        return try {
            block()
        } finally {
            network.serverUrl.set(reachable)
        }
    }

    // --- payloads, in the shapes the API actually serves ---

    protected fun contextJson(
        id: Long,
        name: String = "Work",
    ): String = """{"id":$id,"name":"$name","color":"","isFavourite":false,$STAMPS}"""

    protected fun labelJson(
        id: Long,
        name: String = "bug",
    ): String = """{"id":$id,"name":"$name","color":"","isFavourite":false,"isPrivate":false,$STAMPS}"""

    protected fun projectJson(
        id: Long,
        contextId: Long,
        title: String = "Website",
        labels: List<String> = emptyList(),
    ): String =
        """{"id":$id,"contextId":$contextId,"title":"$title","description":"","color":"",""" +
            """"status":"open","projectType":"generic","isPinned":false,"isPrivate":false,""" +
            """"labels":[${labels.joinToString(",")}],$STAMPS}"""

    protected fun sectionJson(
        id: Long,
        projectId: Long,
        title: String = "Doing",
        position: Int = 0,
    ): String = """{"id":$id,"projectId":$projectId,"title":"$title","position":$position,$STAMPS}"""

    protected fun taskJson(
        id: Long,
        title: String = "Write it down",
        projectId: Long? = null,
        sectionId: Long? = null,
        parentId: Long? = null,
        contextId: Long? = null,
        sourceTaskId: Long? = null,
        status: String = "open",
        planState: String = "none",
        labels: List<String> = emptyList(),
        completedAt: String? = null,
    ): String =
        """{"id":$id,"title":"$title","description":"",""" +
            """"projectId":${projectId ?: "null"},"sectionId":${sectionId ?: "null"},""" +
            """"parentId":${parentId ?: "null"},"contextId":${contextId ?: "null"},""" +
            """"sourceTaskId":${sourceTaskId ?: "null"},"priority":"none","status":"$status",""" +
            """"dayPart":"none","planState":"$planState","isPinned":false,"isPrivate":false,""" +
            """"isComplex":false,"postponeCount":0,"labels":[${labels.joinToString(",")}],""" +
            (if (completedAt == null) "" else """"completedAt":"$completedAt",""") +
            """"blockedByCount":0,"relationCount":0,$STAMPS}"""

    protected fun relationJson(
        id: Long,
        sourceTaskId: Long,
        targetTaskId: Long,
        type: String = "blocks",
    ): String =
        """{"id":$id,"sourceTaskId":$sourceTaskId,"targetTaskId":$targetTaskId,""" +
            """"type":"$type","createdAt":"$WIRE_NOW"}"""

    protected fun templateJson(
        id: Long,
        name: String = "Weekly review",
        subtasks: List<String> = emptyList(),
    ): String =
        """{"id":$id,"name":"$name","description":"","priority":"none","dayPart":"none",""" +
            """"position":0,"labels":[],"subtasks":[${subtasks.joinToString(",")}],$STAMPS}"""

    protected fun templateSubtaskJson(
        id: Long,
        title: String,
    ): String = """{"id":$id,"title":"$title","description":"","priority":"none","dayPart":"none","labels":[]}"""

    /** A complete copy, with only the collections a case cares about filled in. */
    @Suppress("LongParameterList")
    protected fun snapshotJson(
        epoch: Long = 1,
        cursor: Long = 100,
        contexts: List<String> = emptyList(),
        labels: List<String> = emptyList(),
        projects: List<String> = emptyList(),
        sections: List<String> = emptyList(),
        tasks: List<String> = emptyList(),
        relations: List<String> = emptyList(),
        templates: List<String> = emptyList(),
        userSettings: String = """{"locale":"en"}""",
        appSettings: String = """{"autoLabels":[],"projectSuggestions":[]}""",
        userState: String = """{"activeContextId":null}""",
    ): String =
        """
        {"epoch":$epoch,"cursor":$cursor,"completedSince":"$WIRE_NOW",
         "tasks":[${tasks.joinToString(",")}],
         "projects":[${projects.joinToString(",")}],
         "sections":[${sections.joinToString(",")}],
         "contexts":[${contexts.joinToString(",")}],
         "labels":[${labels.joinToString(",")}],
         "taskRelations":[${relations.joinToString(",")}],
         "taskTemplates":[${templates.joinToString(",")}],
         "userSettings":$userSettings,"appSettings":$appSettings,"userState":$userState}
        """.trimIndent()

    protected fun changesJson(
        changes: List<String>,
        epoch: Long = 1,
        cursor: Long = 200,
        hasMore: Boolean = false,
    ): String = """{"epoch":$epoch,"cursor":$cursor,"hasMore":$hasMore,"changes":[${changes.joinToString(",")}]}"""

    protected fun upsert(
        entity: String,
        seq: Long,
        id: Long,
        data: String,
    ): String = """{"entity":"$entity","op":"upsert","seq":$seq,"id":$id,"data":$data}"""

    protected fun tombstone(
        entity: String,
        seq: Long,
        id: Long,
    ): String = """{"entity":"$entity","op":"delete","seq":$seq,"id":$id}"""

    // --- what the replica ended up holding ---

    protected suspend fun queueWrite(
        entity: ReplicaEntityKind,
        localId: Long,
        op: String = "task.patch",
        payload: String = """{"title":"mine"}""",
    ): OutboxOpRow =
        db.outbox().enqueue(
            OutboxOpRow(
                id = "op-$entity-$localId",
                op = op,
                payload = payload,
                entity = entity,
                entityLocalId = localId,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )

    protected suspend fun serverTaskIds(): List<Long> = db.tasks().knownServerIds().sorted()

    protected suspend fun titleOfTask(serverId: Long): String? = db.tasks().byServerId(serverId)?.title

    companion object {
        /** The one moment every fixture is stamped with: 2024-03-09T12:34:56.789Z. */
        const val NOW: Long = 1_709_987_696_789L

        val WIRE_NOW: String = WireTime.format(NOW)

        private val STAMPS: String = """"createdAt":"$WIRE_NOW","updatedAt":"$WIRE_NOW""""

        /** Port 1 is reserved and never listened on, so a connection there is refused at once. */
        private const val UNREACHABLE_ADDRESS: String = "http://127.0.0.1:1"
    }
}
