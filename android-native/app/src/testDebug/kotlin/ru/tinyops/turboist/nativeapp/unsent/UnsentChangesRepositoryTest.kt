package ru.tinyops.turboist.nativeapp.unsent

import android.app.Application
import androidx.room.Room
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.ProjectSectionRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.sync.drain.UnsentChanges
import ru.tinyops.turboist.core.sync.write.OutboxOpKind
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The queue, read back as sentences instead of as rows.
 *
 * Everything above this layer is handed changes that already carry a name, so
 * this is the only place where "Complete task" becomes "Complete task — Renew
 * the domain", and the only place a wrong lookup can hide: a title taken from
 * the wrong table, or a blocker id matched against local ids instead of the
 * server's, reads perfectly well and names the wrong work.
 *
 * So the replica here is real SQLite with the app's own schema, and the ids are
 * deliberately made to disagree — a row's local id is never its server id — so
 * that a lookup against the wrong column cannot accidentally be right.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], application = Application::class)
class UnsentChangesRepositoryTest {
    private lateinit var db: TurboistDatabase
    private lateinit var repository: UnsentChangesRepository

    @Before
    fun openReplica() {
        db =
            Room.inMemoryDatabaseBuilder(RuntimeEnvironment.getApplication(), TurboistDatabase::class.java)
                .build()
        repository = UnsentChangesRepository(db, UnsentChanges(db))
    }

    @After
    fun closeReplica() {
        db.close()
    }

    @Test
    fun `a queued change is named after the task it was made against`() =
        runTest {
            val task = insertTask("Renew the domain")
            enqueue(OutboxOpKind.TASK_COMPLETE, ReplicaEntityKind.TASK, task)

            val change = repository.observeWaiting().first().single()

            assertEquals(OutboxOpKind.TASK_COMPLETE, change.kind)
            assertEquals("Renew the domain", change.target)
            assertNull(change.reason, "a change still in the queue has not been refused")
        }

    @Test
    fun `each kind of row is looked up in the table that holds it`() =
        runTest {
            val context =
                db.contexts().insert(
                    ContextRow(serverId = 71, name = "Household", createdAt = NOW, updatedAt = NOW),
                )
            val project =
                db.projects().insert(
                    ProjectRow(
                        serverId = 72,
                        contextLocalId = context,
                        title = "Renovation",
                        createdAt = NOW,
                        updatedAt = NOW,
                    ),
                )
            val section =
                db.sections().insert(
                    ProjectSectionRow(
                        serverId = 73,
                        projectLocalId = project,
                        title = "Doing",
                        createdAt = NOW,
                        updatedAt = NOW,
                    ),
                )
            val label = db.labels().insert(LabelRow(serverId = 74, name = "errand", createdAt = NOW, updatedAt = NOW))
            val template =
                db.taskTemplates().insert(
                    TaskTemplateRow(serverId = 75, name = "Weekly review", createdAt = NOW, updatedAt = NOW),
                )
            enqueue(OutboxOpKind.PROJECT_PATCH, ReplicaEntityKind.PROJECT, project)
            enqueue(OutboxOpKind.SECTION_PATCH, ReplicaEntityKind.SECTION, section)
            enqueue(OutboxOpKind.CONTEXT_PATCH, ReplicaEntityKind.CONTEXT, context)
            enqueue(OutboxOpKind.LABEL_PATCH, ReplicaEntityKind.LABEL, label)
            enqueue(OutboxOpKind.TEMPLATE_REPLACE, ReplicaEntityKind.TASK_TEMPLATE, template)

            val named = repository.observeWaiting().first().map { it.kind to it.target }

            assertEquals(
                listOf(
                    OutboxOpKind.PROJECT_PATCH to "Renovation",
                    OutboxOpKind.SECTION_PATCH to "Doing",
                    OutboxOpKind.CONTEXT_PATCH to "Household",
                    OutboxOpKind.LABEL_PATCH to "errand",
                    OutboxOpKind.TEMPLATE_REPLACE to "Weekly review",
                ),
                named,
            )
        }

    @Test
    fun `a change to something nobody names reads by its kind alone`() =
        runTest {
            enqueue(OutboxOpKind.SETTINGS_PATCH, ReplicaEntityKind.USER_SETTINGS, SINGLE_ROW)
            enqueue(OutboxOpKind.STATE_PATCH, ReplicaEntityKind.USER_STATE, SINGLE_ROW)
            enqueue(OutboxOpKind.APP_SETTINGS_AUTO_LABELS, ReplicaEntityKind.APP_SETTINGS, SINGLE_ROW)
            enqueue(OutboxOpKind.TASK_RELATION_ADD, ReplicaEntityKind.TASK_RELATION, SINGLE_ROW)

            val changes = repository.observeWaiting().first()

            assertEquals(4, changes.size)
            assertTrue(changes.all { it.target == null }, "an unnamed document was given a name: $changes")
        }

    @Test
    fun `a change aimed at a row the replica no longer holds carries no invented name`() =
        runTest {
            val task = insertTask("Cancelled outing")
            enqueue(OutboxOpKind.TASK_PATCH, ReplicaEntityKind.TASK, task)
            db.tasks().deleteByLocalIds(listOf(task))

            assertNull(repository.observeWaiting().first().single().target)
        }

    @Test
    fun `a row with a blank title reads as unnamed rather than as an empty line`() =
        runTest {
            val task = insertTask("   ")
            enqueue(OutboxOpKind.TASK_PATCH, ReplicaEntityKind.TASK, task)

            assertNull(repository.observeWaiting().first().single().target)
        }

    @Test
    fun `a refused change carries the reason the server gave and the name it was for`() =
        runTest {
            val task = insertTask("Ship the release")
            quarantine(
                OutboxOpKind.TASK_COMPLETE,
                ReplicaEntityKind.TASK,
                task,
                errorCode = ApiErrorCodes.TARGET_GONE,
            )

            val change = repository.observeSetAside().first().single()

            assertEquals(OutboxOpKind.TASK_COMPLETE, change.kind)
            assertEquals("Ship the release", change.target)
            assertEquals(UnsentReason.TARGET_GONE, change.reason)
            assertTrue(repository.observeWaiting().first().isEmpty(), "a refusal is out of the queue")
        }

    @Test
    fun `the ids a refusal named are resolved into the titles of the work in the way`() =
        runTest {
            val blocked = insertTask("Ship the release", serverId = 900)
            insertTask("Pay the invoice", serverId = 901)
            insertTask("Sign the contract", serverId = 902)
            quarantine(
                OutboxOpKind.TASK_COMPLETE,
                ReplicaEntityKind.TASK,
                blocked,
                errorCode = ApiErrorCodes.TASK_BLOCKED,
                blockedBy = listOf(901, 902),
            )

            val change = repository.observeSetAside().first().single()

            assertEquals(UnsentReason.BLOCKED, change.reason)
            assertEquals(listOf("Pay the invoice", "Sign the contract"), change.blockers)
        }

    @Test
    fun `a blocker the replica does not hold is left out rather than shown as a number`() =
        runTest {
            // The local ids run from 1, so an id that is only ever a local id
            // resolves to nothing when it is read as a server id — which is what
            // makes this case fail if the lookup uses the wrong column.
            val blocked = insertTask("Ship the release", serverId = 900)
            insertTask("Pay the invoice", serverId = 901)
            quarantine(
                OutboxOpKind.TASK_COMPLETE,
                ReplicaEntityKind.TASK,
                blocked,
                errorCode = ApiErrorCodes.TASK_BLOCKED,
                blockedBy = listOf(901, 1, 2),
            )

            assertEquals(listOf("Pay the invoice"), repository.observeSetAside().first().single().blockers)
        }

    @Test
    fun `a change queued by a build this one does not know is still listed`() =
        runTest {
            val task = insertTask("Renew the domain")
            db.outbox().enqueue(
                OutboxOpRow(
                    id = "op-future",
                    op = "task.teleport",
                    payload = "{}",
                    entity = ReplicaEntityKind.TASK,
                    entityLocalId = task,
                    createdAt = NOW,
                    updatedAt = NOW,
                ),
            )

            val change = repository.observeWaiting().first().single()

            assertNull(change.kind, "an op name from a later build is not one of this build's")
            assertEquals("Renew the domain", change.target, "and the change is still named")
        }

    @Test
    fun `discarding one refusal closes that note and leaves the rest`() =
        runTest {
            val task = insertTask("Ship the release")
            quarantine(OutboxOpKind.TASK_COMPLETE, ReplicaEntityKind.TASK, task, id = "op-a")
            quarantine(OutboxOpKind.TASK_PATCH, ReplicaEntityKind.TASK, task, id = "op-b")

            assertTrue(repository.discard("op-a"))

            assertEquals(listOf("op-b"), repository.observeSetAside().first().map { it.id })
        }

    @Test
    fun `discarding everything reports how many notes there were`() =
        runTest {
            val task = insertTask("Ship the release")
            quarantine(OutboxOpKind.TASK_COMPLETE, ReplicaEntityKind.TASK, task, id = "op-a")
            quarantine(OutboxOpKind.TASK_PATCH, ReplicaEntityKind.TASK, task, id = "op-b")

            assertEquals(2, repository.discardAll())

            assertTrue(repository.observeSetAside().first().isEmpty())
        }

    private suspend fun insertTask(
        title: String,
        serverId: Long? = null,
    ): Long = db.tasks().insert(TaskRow(serverId = serverId, title = title, createdAt = NOW, updatedAt = NOW))

    private suspend fun enqueue(
        kind: OutboxOpKind,
        entity: ReplicaEntityKind,
        localId: Long,
    ) {
        db.outbox().enqueue(
            OutboxOpRow(
                id = "op-${kind.stored}-$localId",
                op = kind.stored,
                payload = "{}",
                entity = entity,
                entityLocalId = localId,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )
    }

    private suspend fun quarantine(
        kind: OutboxOpKind,
        entity: ReplicaEntityKind,
        localId: Long,
        id: String = "op-${kind.stored}-$localId",
        errorCode: String = ApiErrorCodes.CONFLICT,
        blockedBy: List<Long> = emptyList(),
    ) {
        val queued =
            db.outbox().enqueue(
                OutboxOpRow(
                    id = id,
                    op = kind.stored,
                    payload = "{}",
                    entity = entity,
                    entityLocalId = localId,
                    createdAt = NOW,
                    updatedAt = NOW,
                ),
            )
        db.outbox().quarantine(
            op = queued,
            errorCode = errorCode,
            errorMessage = "",
            httpStatus = null,
            quarantinedAt = NOW,
            blockedBy = blockedBy,
        )
    }

    private companion object {
        /** 2024-03-09T12:34:56.789Z, the instant the rest of the replica's tests are written against. */
        const val NOW: Long = 1_709_987_696_789L

        /** The local id of a document there is only ever one of. */
        const val SINGLE_ROW: Long = 1L
    }
}
