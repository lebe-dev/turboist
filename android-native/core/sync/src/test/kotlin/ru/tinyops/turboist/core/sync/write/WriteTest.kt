package ru.tinyops.turboist.core.sync.write

import androidx.room.Room
import org.junit.After
import org.junit.Before
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.AppSettingsRow
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.TaskRelationRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.entity.UserSettingsRow
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import java.time.ZoneId

/**
 * A real database and the real write path over it.
 *
 * The claim these tests exist to check — that a change to the replica and the
 * request that will report it are one indivisible step — is a claim about a
 * transaction engine. A stub would agree with whatever the code did, which is
 * precisely the thing under test, so every test here runs against SQLite.
 *
 * Time and op identities are given rather than observed: a test that asserts on
 * `now()` is asserting on when it happened to run.
 */
@RunWith(RobolectricTestRunner::class)
abstract class WriteTest {
    protected lateinit var db: TurboistDatabase
    protected lateinit var writer: OutboxWriter
    protected lateinit var rules: ReplicaRules
    protected lateinit var tasks: TaskWriteRepo
    protected lateinit var projects: ProjectWriteRepo
    protected lateinit var contexts: ContextWriteRepo
    protected lateinit var labels: LabelWriteRepo
    protected lateinit var templates: TemplateWriteRepo
    protected lateinit var settings: SettingsWriteRepo
    protected lateinit var troiki: TroikiWriteRepo

    /** The moment every write in a test is made at, unless a test moves it. */
    protected var clock: Long = NOW

    private var opCounter: Int = 0

    /**
     * An op identity to hand out instead of a fresh one.
     *
     * Set by the test that needs two writes to collide, which is the only way to
     * make the queue reject an op from inside a transaction that has already
     * changed the replica.
     */
    protected var forcedOpId: String? = null

    @Before
    fun openReplica() {
        db =
            Room.inMemoryDatabaseBuilder(RuntimeEnvironment.getApplication(), TurboistDatabase::class.java)
                .build()
        opCounter = 0
        clock = NOW
        forcedOpId = null
        writer = OutboxWriter(db, { clock }, { forcedOpId ?: "op-${++opCounter}" })
        rules = ReplicaRules(db)
        tasks = TaskWriteRepo(db, writer, rules, RecurrenceAdvancer(ZONE), ZONE)
        projects = ProjectWriteRepo(db, writer, rules)
        contexts = ContextWriteRepo(db, writer)
        labels = LabelWriteRepo(db, writer)
        templates = TemplateWriteRepo(db, writer)
        settings = SettingsWriteRepo(db, writer)
        troiki = TroikiWriteRepo(db, writer)
    }

    @After
    fun closeReplica() {
        db.close()
    }

    // --- fixtures ------------------------------------------------------------

    protected suspend fun givenContext(
        name: String = "Work",
        serverId: Long? = 1L,
    ): Long = db.contexts().insert(ContextRow(serverId = serverId, name = name, createdAt = NOW, updatedAt = NOW))

    protected suspend fun givenProject(
        contextLocalId: Long,
        title: String = "Website",
        serverId: Long? = 1L,
        troikiCategory: TroikiCategory? = null,
    ): Long =
        db.projects().insert(
            ProjectRow(
                serverId = serverId,
                contextLocalId = contextLocalId,
                title = title,
                troikiCategory = troikiCategory,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )

    protected suspend fun givenLabel(
        name: String = "bug",
        serverId: Long? = 1L,
    ): Long = db.labels().insert(LabelRow(serverId = serverId, name = name, createdAt = NOW, updatedAt = NOW))

    protected suspend fun givenTask(
        title: String = "Write it down",
        serverId: Long? = 1L,
        projectLocalId: Long? = null,
        parentLocalId: Long? = null,
        inboxId: Long? = null,
        dueAt: Long? = null,
        recurrenceRule: String? = null,
        priority: Priority = Priority.NONE,
        status: TaskStatus = TaskStatus.OPEN,
    ): Long =
        db.tasks().insert(
            TaskRow(
                serverId = serverId,
                title = title,
                projectLocalId = projectLocalId,
                parentLocalId = parentLocalId,
                inboxId = inboxId,
                dueAt = dueAt,
                recurrenceRule = recurrenceRule,
                priority = priority,
                status = status,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )

    /** [blocker] must be finished before [blocked] can be. */
    protected suspend fun givenBlocks(
        blocker: Long,
        blocked: Long,
    ): Long =
        db.taskRelations().insert(
            TaskRelationRow(
                sourceTaskLocalId = blocker,
                targetTaskLocalId = blocked,
                type = RelationType.BLOCKS,
                createdAt = NOW,
            ),
        )

    protected suspend fun givenUserSettings(payload: String) {
        db.settings().saveUserSettings(UserSettingsRow(payload = payload, updatedAt = NOW))
    }

    protected suspend fun givenAppSettings(payload: String) {
        db.settings().saveAppSettings(AppSettingsRow(payload = payload, updatedAt = NOW))
    }

    protected suspend fun queue(): List<OutboxOpRow> = db.outbox().all()

    protected suspend fun queuedOps(): List<OutboxOp> = queue().map { OutboxOpCodec.decode(it.payload) }

    companion object {
        /** 2024-03-09T12:34:56.789Z, the instant the wire format is written against. */
        const val NOW: Long = 1_709_987_696_789L

        /**
         * The clock the write path is tested against.
         *
         * Fixed rather than the machine's, because a repeating task is moved on
         * by a rule that repeats a wall clock: a case written in one zone and run
         * in another would be asking a different question.
         */
        val ZONE: ZoneId = ZoneId.of("UTC")
    }
}
