package ru.tinyops.turboist.core.database

import android.database.Cursor
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Test
import java.io.File
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * The checked-in description of the schema, held to the schema itself.
 *
 * The build writes a JSON description of the database next to this module and it
 * is committed. That file is not documentation: it is the starting point a
 * migration is written and tested against, so when the replica moves to the next
 * version there has to be an accurate account of the version it is moving from.
 * An export that is missing, misnumbered, or describing something other than what
 * the engine builds would only be discovered at that point — by which time the
 * migration has already been written against a fiction.
 *
 * So the file is read back here and checked against the live database, which is
 * the same comparison the runtime makes on a device when it opens an existing
 * file: the identity hash covers every table, column, index and foreign key, and
 * a single-character difference changes it.
 */
class SchemaExportTest : ReplicaTest() {
    @Test
    fun `the current version is exported`() {
        assertTrue(
            exportFile.isFile,
            "no export for version ${TurboistDatabase.VERSION} at ${exportFile.path} — " +
                "the next migration would have nothing to start from",
        )
    }

    @Test
    fun `the export is numbered as the version it describes`() {
        assertEquals(
            TurboistDatabase.VERSION,
            exportedDatabase["version"]?.jsonPrimitive?.content?.toInt(),
            "the export's file name and its contents must name the same version",
        )
    }

    @Test
    fun `the export describes the database the engine builds`() {
        val stamped =
            assertNotNull(
                query("SELECT identity_hash FROM room_master_table") { it.getString(0) }.firstOrNull(),
                "the database records the identity of the schema it was created from",
            )

        assertEquals(
            exportedDatabase["identityHash"]?.jsonPrimitive?.content,
            stamped,
            "the exported schema and the built schema have drifted; re-export before writing a migration",
        )
    }

    @Test
    fun `every table in the database is accounted for in the export`() {
        val exported =
            exportedDatabase
                .getValue("entities")
                .jsonArray
                .map { it.jsonObject.getValue("tableName").jsonPrimitive.content }
                .toSet()

        // SQLite keeps housekeeping tables of its own, and the full-text indexes
        // each expand into several shadow tables that belong to the index rather
        // than to the schema. Neither is ours to export.
        val live =
            query("SELECT name FROM sqlite_master WHERE type = 'table'") { it.getString(0) }
                .filterNot { it.startsWith("sqlite_") || it == "room_master_table" || it == "android_metadata" }
                .filterNot { name -> exported.any { name.startsWith("${it}_") } }
                .toSet()

        assertEquals(emptySet(), live - exported, "a table was added without re-exporting the schema")
    }

    private val exportFile: File
        get() =
            File(System.getProperty("turboist.schemaExport.dir").orEmpty())
                .resolve(TurboistDatabase::class.java.canonicalName!!)
                .resolve("${TurboistDatabase.VERSION}.json")

    private val exportedDatabase: JsonObject
        get() = Json.parseToJsonElement(exportFile.readText()).jsonObject.getValue("database").jsonObject

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
