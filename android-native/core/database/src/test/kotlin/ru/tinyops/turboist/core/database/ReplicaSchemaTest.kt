package ru.tinyops.turboist.core.database

import android.database.Cursor
import org.junit.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * What the engine actually built.
 *
 * The tests above prove behaviour; this one pins the structure that behaviour
 * rests on, so a change to it has to be a deliberate edit here rather than a
 * surprise on a device. It reads the schema back out of SQLite rather than out
 * of the annotations, because the annotations are the request and this is the
 * answer.
 */
class ReplicaSchemaTest : ReplicaTest() {
    @Test
    fun `referential integrity is switched on`() {
        val enabled = query("PRAGMA foreign_keys") { it.getInt(0) }.single()
        assertEquals(1, enabled, "without this every foreign key in the schema is decoration")
    }

    @Test
    fun `the replica holds the workspace, its indexes and its bookkeeping`() {
        val tables = query("SELECT name FROM sqlite_master WHERE type = 'table'") { it.getString(0) }.toSet()

        val workspace =
            setOf(
                "contexts", "labels", "projects", "project_labels", "project_sections",
                "tasks", "task_labels", "task_relations",
                "task_templates", "task_template_subtasks",
                "task_template_labels", "task_template_subtask_labels",
                "user_settings", "app_settings", "user_state",
            )
        val indexes = setOf("tasks_fts", "projects_fts", "labels_fts", "contexts_fts")
        val bookkeeping = setOf("sync_state", "outbox", "quarantine")

        assertTrue(tables.containsAll(workspace), "missing: ${workspace - tables}")
        assertTrue(tables.containsAll(indexes), "missing: ${indexes - tables}")
        assertTrue(tables.containsAll(bookkeeping), "missing: ${bookkeeping - tables}")
    }

    @Test
    fun `a record can be resolved by its server id, and only one row ever answers`() {
        val resolvable =
            listOf(
                "contexts",
                "labels",
                "projects",
                "project_sections",
                "tasks",
                "task_relations",
                "task_templates",
            )
        for (table in resolvable) {
            val unique =
                query("PRAGMA index_list(`$table`)") { it.getString(1) to it.getInt(2) }
                    .filter { (_, isUnique) -> isUnique == 1 }
                    .map { (name, _) -> name }
                    .any { indexName ->
                        query("PRAGMA index_info(`$indexName`)") { it.getString(2) } == listOf("serverId")
                    }
            assertTrue(unique, "$table must resolve an incoming record by server id, without ambiguity")
        }
    }

    @Test
    fun `a task follows its placement exactly as far as the server does`() {
        val actions = foreignKeyActions("tasks")

        assertEquals("CASCADE", actions["contextLocalId"], "a context takes its tasks with it")
        assertEquals("CASCADE", actions["projectLocalId"], "so does a project")
        assertEquals("SET NULL", actions["sectionLocalId"], "a board column does not: its tasks stay in the project")
        assertEquals("CASCADE", actions["parentLocalId"], "a task takes its subtasks with it")
        assertEquals("SET NULL", actions["sourceTaskLocalId"], "a recurrence snapshot outlives the task it came from")
    }

    @Test
    fun `join rows never outlive either of their ends`() {
        assertEquals("CASCADE", foreignKeyActions("task_labels")["taskLocalId"])
        assertEquals("CASCADE", foreignKeyActions("task_labels")["labelLocalId"])
        assertEquals("CASCADE", foreignKeyActions("project_labels")["projectLocalId"])
        assertEquals("CASCADE", foreignKeyActions("project_labels")["labelLocalId"])
        assertEquals("CASCADE", foreignKeyActions("task_relations")["sourceTaskLocalId"])
        assertEquals("CASCADE", foreignKeyActions("task_relations")["targetTaskLocalId"])
        assertEquals("CASCADE", foreignKeyActions("task_template_subtasks")["templateLocalId"])
    }

    @Test
    fun `nothing local ever leaks a foreign key into the replicated tables`() {
        // The queue and the quarantine outlive the rows they name — a write
        // whose target was deleted on the server has to survive long enough to
        // be shown to the user as refused.
        assertTrue(foreignKeyActions("outbox").isEmpty())
        assertTrue(foreignKeyActions("quarantine").isEmpty())
        assertTrue(foreignKeyActions("sync_state").isEmpty())
    }

    /** Which column of [table] points where, and what happens when the target goes. */
    private fun foreignKeyActions(table: String): Map<String, String> =
        query("PRAGMA foreign_key_list(`$table`)") { it.getString(3) to it.getString(6) }.toMap()

    private fun <T> query(
        sql: String,
        read: (Cursor) -> T,
    ): List<T> {
        val results = mutableListOf<T>()
        assertNotNull(db.openHelper.readableDatabase.query(sql)).use { cursor ->
            while (cursor.moveToNext()) results += read(cursor)
        }
        return results
    }
}
