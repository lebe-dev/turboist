package ru.tinyops.turboist.core.database

import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * What the label usage report is read from.
 *
 * The report is counted on the device, so every fact it needs has to be a fact
 * the replica holds: when the label was applied, and the state, dates and
 * project of the task carrying it. The cases below pin exactly that, because a
 * column quietly dropped from the join would not fail anything — it would make
 * the report wrong by a number nobody can check against.
 */
class LabelUsageQueryTest : ReplicaTest() {
    @Test
    fun `a tagging carries the moment it was made and the facts of its task`() =
        runTest {
            val projectLocalId = db.projects().insert(project(db.contexts().insert(context())))
            val labelLocalId = db.labels().insert(label(name = "urgent"))
            val taskLocalId =
                db.tasks().insert(
                    task(projectLocalId = projectLocalId).copy(
                        status = TaskStatus.COMPLETED,
                        dueAt = NOW - 86_400_000,
                        completedAt = NOW,
                    ),
                )
            db.tasks().setLabels(taskLocalId, listOf(labelLocalId), taggedAt = NOW - 172_800_000)

            val facts = db.labelUsage().observeTaggings().first().single()

            assertEquals(labelLocalId, facts.labelLocalId)
            assertEquals(NOW - 172_800_000, facts.taggedAt, "the tagging's own moment, not the task's")
            assertEquals(TaskStatus.COMPLETED, facts.status)
            assertEquals(NOW - 86_400_000, facts.dueAt)
            assertEquals(NOW, facts.completedAt)
            assertEquals(projectLocalId, facts.projectLocalId)
        }

    @Test
    fun `work filed nowhere reports no project rather than being left out`() =
        runTest {
            val labelLocalId = db.labels().insert(label())
            val taskLocalId = db.tasks().insert(task())
            db.tasks().setLabels(taskLocalId, listOf(labelLocalId), taggedAt = NOW)

            val facts = db.labelUsage().observeTaggings().first().single()

            assertNull(facts.projectLocalId, "an inbox task counts as tagged, and towards no project")
        }

    @Test
    fun `a label on several tasks is reported once per task`() =
        runTest {
            val labelLocalId = db.labels().insert(label())
            repeat(3) { index ->
                val taskLocalId = db.tasks().insert(task(title = "task $index"))
                db.tasks().setLabels(taskLocalId, listOf(labelLocalId), taggedAt = NOW + index)
            }

            val facts = db.labelUsage().observeTaggings().first()

            assertEquals(3, facts.size)
            assertTrue(facts.all { it.labelLocalId == labelLocalId })
        }

    @Test
    fun `deleting the task takes its taggings out of the report`() =
        runTest {
            val labelLocalId = db.labels().insert(label())
            val taskRow = task()
            val taskLocalId = db.tasks().insert(taskRow)
            db.tasks().setLabels(taskLocalId, listOf(labelLocalId), taggedAt = NOW)

            db.tasks().delete(taskRow.copy(localId = taskLocalId))

            assertEquals(emptyList(), db.labelUsage().observeTaggings().first())
        }
}
