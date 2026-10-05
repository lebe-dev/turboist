package ru.tinyops.turboist.core.sync.drain

import androidx.room.Room
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Before
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.entity.QuarantinedOpRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.ServerUrl
import ru.tinyops.turboist.core.network.TurboistNetwork
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.SyncMutex
import ru.tinyops.turboist.core.sync.pull.ReplicaApplier
import ru.tinyops.turboist.core.sync.pull.SyncPuller
import ru.tinyops.turboist.core.sync.write.OutboxWriter
import ru.tinyops.turboist.core.sync.write.ReplicaServerIds
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import ru.tinyops.turboist.core.sync.write.WriteClock

/**
 * The write half of a sync cycle, end to end.
 *
 * Both ends are real: the queue is written by the same repositories a screen
 * uses, into SQLite with its foreign keys live, and it drains over the app's own
 * HTTP stack into a server that remembers what it has been told. Nothing in
 * between is stubbed, because nearly everything the drainer can get wrong —
 * sending a write under a fresh key, sending a local id, sending two writes in
 * the wrong order, treating a replayed answer as a failure — is invisible to a
 * test that stands in for either end.
 *
 * Time and op identities are given rather than observed, so a case can talk
 * about "the second write" instead of about whatever UUID happened to be minted.
 */
@RunWith(RobolectricTestRunner::class)
abstract class DrainTest {
    protected lateinit var db: TurboistDatabase
    protected lateinit var server: MockWebServer
    protected lateinit var scripted: ScriptedServer
    protected lateinit var network: TurboistNetwork
    protected lateinit var writer: OutboxWriter
    protected lateinit var tasks: TaskWriteRepo
    protected lateinit var ids: ReplicaServerIds
    protected lateinit var drainer: OutboxDrainer
    protected lateinit var puller: SyncPuller
    protected lateinit var cycle: SyncCycle
    protected lateinit var unsent: UnsentChanges

    private var opCounter: Int = 0

    @Before
    fun openReplicaAndServer() {
        db =
            Room.inMemoryDatabaseBuilder(RuntimeEnvironment.getApplication(), TurboistDatabase::class.java)
                .build()
        scripted = ScriptedServer(NOW)
        server = MockWebServer()
        server.dispatcher = scripted
        server.start()
        network = TurboistNetwork.create(serverUrl = ServerUrl(server.url("/").toString()))
        opCounter = 0
        writer = OutboxWriter(db, { NOW }, { "op-${++opCounter}" })
        tasks = TaskWriteRepo(db, writer)
        ids = ReplicaServerIds(db)
        val mutex = SyncMutex()
        drainer = newDrainer(mutex)
        puller = SyncPuller(network.sync, db, ReplicaApplier(db) { NOW }, mutex)
        cycle = SyncCycle.sending(drainer, puller)
        unsent = UnsentChanges(db)
    }

    @After
    fun closeReplicaAndServer() {
        db.close()
        server.close()
    }

    /** A drainer over the same replica — what a restarted process gets, with its patience reset. */
    protected fun newDrainer(
        mutex: SyncMutex = SyncMutex(),
        serverErrorLimit: Int = OutboxDrainer.DEFAULT_SERVER_ERROR_LIMIT,
    ): OutboxDrainer =
        OutboxDrainer(
            db = db,
            sender = OpSender(network, ids),
            ids = ids,
            mutex = mutex,
            clock = WriteClock { NOW },
            serverErrorLimit = serverErrorLimit,
        )

    protected fun runReplicaTest(block: suspend CoroutineScope.() -> Unit) {
        runBlocking(block = block)
    }

    /**
     * Runs [block] against an address nothing is listening on, which is what
     * having no network looks like from inside the client. The server is still
     * there afterwards, for the half of a case where the connection comes back.
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

    // --- what the replica holds ----------------------------------------------

    protected suspend fun queue(): List<OutboxOpRow> = db.outbox().all()

    protected suspend fun setAside(): List<QuarantinedOpRow> = db.outbox().quarantined()

    protected suspend fun serverIdOfTask(localId: Long): Long? = db.tasks().byLocalId(localId)?.serverId

    /** A task the device already knows the server's name for, as an earlier catch-up would have left it. */
    protected suspend fun givenSyncedTask(
        serverId: Long,
        title: String = "Already there",
    ): Long =
        db.tasks().insert(
            TaskRow(
                serverId = serverId,
                title = title,
                inboxId = ScriptedServer.INBOX_ID,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )

    /** Queues a write by hand, for the cases that are about a payload no repository would write. */
    protected suspend fun queueRaw(
        id: String,
        op: String,
        payload: String,
        entityLocalId: Long,
        entity: ReplicaEntityKind = ReplicaEntityKind.TASK,
    ): OutboxOpRow =
        db.outbox().enqueue(
            OutboxOpRow(
                id = id,
                op = op,
                payload = payload,
                entity = entity,
                entityLocalId = entityLocalId,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )

    companion object {
        /** The one moment every fixture is stamped with: 2024-03-09T12:34:56.789Z. */
        const val NOW: Long = 1_709_987_696_789L

        /** Port 1 is reserved and never listened on, so a connection there is refused at once. */
        private const val UNREACHABLE_ADDRESS: String = "http://127.0.0.1:1"
    }
}
