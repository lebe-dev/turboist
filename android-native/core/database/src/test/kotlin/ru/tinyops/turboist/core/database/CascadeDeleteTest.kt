package ru.tinyops.turboist.core.database

import android.database.sqlite.SQLiteConstraintException
import androidx.room.withTransaction
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.TaskRelationRow
import ru.tinyops.turboist.core.model.RelationType
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * What a delete takes with it.
 *
 * Records are deleted outright on both sides — there is no tombstone column
 * anywhere, so a cascade on the device has to reach exactly as far as the
 * server's does. Reaching less leaves rows the server no longer has and will
 * never mention again; reaching further silently destroys the user's data.
 */
class CascadeDeleteTest : ReplicaTest() {
    @Test
    fun `deleting a context takes its projects and their tasks`() =
        runTest {
            val contextLocalId = db.contexts().upsertByServerId(context(serverId = 1))
            val projectLocalId = db.projects().upsertByServerId(project(contextLocalId, serverId = 2))
            db.sections().upsertByServerId(section(projectLocalId, serverId = 3))
            db.tasks().upsertByServerId(
                task(serverId = 4, contextLocalId = contextLocalId, projectLocalId = projectLocalId),
            )

            db.contexts().deleteByServerId(1)

            assertEquals(0, db.projects().count())
            assertEquals(0, db.sections().count())
            assertEquals(0, db.tasks().count())
        }

    @Test
    fun `deleting a board column keeps its tasks in the project`() =
        runTest {
            val contextLocalId = db.contexts().upsertByServerId(context(serverId = 1))
            val projectLocalId = db.projects().upsertByServerId(project(contextLocalId, serverId = 2))
            val sectionLocalId = db.sections().upsertByServerId(section(projectLocalId, serverId = 3))
            val taskLocalId =
                db.tasks().upsertByServerId(
                    task(
                        serverId = 4,
                        contextLocalId = contextLocalId,
                        projectLocalId = projectLocalId,
                        sectionLocalId = sectionLocalId,
                    ),
                )

            db.sections().deleteByServerId(3)

            val stored = assertNotNull(db.tasks().byLocalId(taskLocalId))
            assertNull(stored.sectionLocalId, "the task loses its column, not its home")
            assertEquals(projectLocalId, stored.projectLocalId)
        }

    @Test
    fun `deleting a task takes its subtasks, all the way down`() =
        runTest {
            val parent = db.tasks().upsertByServerId(task(title = "Parent", serverId = 1))
            val child = db.tasks().upsertByServerId(task(title = "Child", serverId = 2, parentLocalId = parent))
            db.tasks().upsertByServerId(task(title = "Grandchild", serverId = 3, parentLocalId = child))

            db.tasks().deleteByServerId(1)

            assertEquals(0, db.tasks().count())
        }

    @Test
    fun `deleting a recurring task keeps the snapshots cut from it`() =
        runTest {
            val recurring = db.tasks().upsertByServerId(task(title = "Water plants", serverId = 1))
            val snapshot =
                db.tasks().upsertByServerId(
                    task(title = "Water plants", serverId = 2, sourceTaskLocalId = recurring),
                )

            db.tasks().deleteByServerId(1)

            val stored = assertNotNull(db.tasks().byLocalId(snapshot))
            assertNull(stored.sourceTaskLocalId, "the history survives; only the pointer back is gone")
            assertEquals(1, db.tasks().count())
        }

    @Test
    fun `deleting a task takes the edges it is an endpoint of, from either end`() =
        runTest {
            val blocker = db.tasks().upsertByServerId(task(title = "Blocker", serverId = 1))
            val blocked = db.tasks().upsertByServerId(task(title = "Blocked", serverId = 2))
            val bystander = db.tasks().upsertByServerId(task(title = "Bystander", serverId = 3))
            db.taskRelations().insert(
                TaskRelationRow(
                    serverId = 10,
                    sourceTaskLocalId = blocker,
                    targetTaskLocalId = blocked,
                    type = RelationType.BLOCKS,
                    createdAt = NOW,
                ),
            )
            db.taskRelations().insert(
                TaskRelationRow(
                    serverId = 11,
                    sourceTaskLocalId = bystander,
                    targetTaskLocalId = blocker,
                    type = RelationType.RELATED,
                    createdAt = NOW,
                ),
            )

            db.tasks().deleteByServerId(1)

            assertEquals(0, db.taskRelations().count(), "an edge with a missing endpoint is not an edge")
            assertEquals(2, db.tasks().count())
        }

    @Test
    fun `deleting a label detaches it everywhere it was used`() =
        runTest {
            val labelLocalId = db.labels().upsertByServerId(label(serverId = 1))
            val taskLocalId = db.tasks().upsertByServerId(task(serverId = 2))
            db.tasks().setLabels(taskLocalId, listOf(labelLocalId), NOW)

            db.labels().deleteByServerId(1)

            assertTrue(db.tasks().labelsOf(taskLocalId).isEmpty())
            assertEquals(1, db.tasks().count(), "the task itself is untouched")
        }

    @Test
    fun `a reference to a row that is not there is refused on the spot`() =
        runTest {
            // References are checked statement by statement, so whoever applies a
            // batch writes what a task points at before the task. The failure
            // names the statement that made the bad reference rather than
            // arriving as one opaque error at the end of a batch.
            assertFailsWith<SQLiteConstraintException> {
                db.tasks().insert(task(title = "Orphan", parentLocalId = 9_999))
            }
            assertEquals(0, db.tasks().count())

            assertFailsWith<SQLiteConstraintException> {
                db.withTransaction {
                    db.tasks().insert(task(title = "Parent").copy(localId = 10))
                    db.tasks().insert(task(title = "Child", parentLocalId = 10).copy(localId = 20))
                    db.tasks().insert(task(title = "Orphan", parentLocalId = 9_999))
                }
            }
            assertEquals(0, db.tasks().count(), "the whole batch is refused, not the part that fit")
        }
}
