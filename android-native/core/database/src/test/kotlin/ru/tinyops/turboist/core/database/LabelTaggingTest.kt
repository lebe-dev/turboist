package ru.tinyops.turboist.core.database

import kotlinx.coroutines.test.runTest
import org.junit.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Re-labelling a task is a difference, not a rewrite.
 *
 * The moment a label was attached is the only data the label usage report is
 * computed from, and re-stamping an edge that never moved would be invisible on
 * screen while quietly destroying that history — which is why the server diffs
 * too.
 */
class LabelTaggingTest : ReplicaTest() {
    @Test
    fun `a label that stays on the task keeps the moment it was attached`() =
        runTest {
            val taskLocalId = db.tasks().insert(task())
            val keep = db.labels().insert(label(name = "bug"))
            val dropped = db.labels().insert(label(name = "chore"))
            val added = db.labels().insert(label(name = "urgent"))

            db.tasks().setLabels(taskLocalId, listOf(keep, dropped), taggedAt = NOW)
            db.tasks().setLabels(taskLocalId, listOf(keep, added), taggedAt = NOW + 86_400_000)

            val edges = db.tasks().labelsOf(taskLocalId).associateBy { it.labelLocalId }
            assertEquals(setOf(keep, added), edges.keys)
            assertEquals(NOW, edges.getValue(keep).createdAt, "this label was applied a day ago, not now")
            assertEquals(NOW + 86_400_000, edges.getValue(added).createdAt)
        }

    @Test
    fun `clearing every label leaves no edge behind`() =
        runTest {
            val taskLocalId = db.tasks().insert(task())
            val labelLocalId = db.labels().insert(label())
            db.tasks().setLabels(taskLocalId, listOf(labelLocalId), taggedAt = NOW)

            db.tasks().setLabels(taskLocalId, emptyList(), taggedAt = NOW + 1)

            assertTrue(db.tasks().labelsOf(taskLocalId).isEmpty())
        }

    @Test
    fun `the same label named twice is attached once`() =
        runTest {
            val taskLocalId = db.tasks().insert(task())
            val labelLocalId = db.labels().insert(label())

            db.tasks().setLabels(taskLocalId, listOf(labelLocalId, labelLocalId), taggedAt = NOW)

            assertEquals(1, db.tasks().labelsOf(taskLocalId).size)
        }
}
