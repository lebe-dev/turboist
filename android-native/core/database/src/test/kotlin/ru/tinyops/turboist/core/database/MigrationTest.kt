package ru.tinyops.turboist.core.database

import android.database.sqlite.SQLiteDatabase
import androidx.room.Room
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import java.io.File
import kotlin.test.assertEquals

/**
 * A replica written by an older build opens in this one with its rows intact.
 *
 * The older file is rebuilt from that version's checked-in schema export, which
 * is exactly what a device carries, and is then handed to Room at the current
 * version. Room validates the migrated schema against the one it expects, so an
 * incomplete migration fails here rather than on a device.
 */
@RunWith(RobolectricTestRunner::class)
class MigrationTest {
    @Test
    fun `version 1 labels gain an empty description`() =
        runTest {
            val app = RuntimeEnvironment.getApplication()
            val file = app.getDatabasePath("migration-v1.db")
            file.parentFile?.mkdirs()
            file.delete()

            SQLiteDatabase.openOrCreateDatabase(file, null).use { legacy ->
                createFromExport(legacy, version = 1)
                legacy.execSQL(
                    "INSERT INTO labels (serverId, name, color, isFavourite, isPrivate, createdAt, updatedAt) " +
                        "VALUES (7, 'bug', 'red', 0, 0, 1, 1)",
                )
                legacy.version = 1
            }

            val db =
                Room.databaseBuilder(app, TurboistDatabase::class.java, file.absolutePath)
                    .allowMainThreadQueries()
                    .build()
            try {
                val label = db.labels().byServerId(7)
                assertEquals("bug", label?.name)
                assertEquals("", label?.description)
            } finally {
                db.close()
                file.delete()
            }
        }

    @Test
    fun `version 2 projects are not exempt from the today lock`() =
        runTest {
            val app = RuntimeEnvironment.getApplication()
            val file = app.getDatabasePath("migration-v2.db")
            file.parentFile?.mkdirs()
            file.delete()

            SQLiteDatabase.openOrCreateDatabase(file, null).use { legacy ->
                createFromExport(legacy, version = 2)
                legacy.execSQL(
                    "INSERT INTO contexts (serverId, name, color, isFavourite, createdAt, updatedAt) " +
                        "VALUES (1, 'work', 'blue', 0, 1, 1)",
                )
                legacy.execSQL(
                    "INSERT INTO projects (serverId, contextLocalId, title, description, color, status, type, " +
                        "isPinned, isPrivate, createdAt, updatedAt) " +
                        "VALUES (7, 1, 'Home', '', '', 'open', 'generic', 0, 0, 1, 1)",
                )
                legacy.version = 2
            }

            val db =
                Room.databaseBuilder(app, TurboistDatabase::class.java, file.absolutePath)
                    .allowMainThreadQueries()
                    .build()
            try {
                val project = db.projects().byServerId(7)
                assertEquals("Home", project?.title)
                assertEquals(false, project?.todayLockExempt)
            } finally {
                db.close()
                file.delete()
            }
        }

    @Test
    fun `version 3 keeps its rows and gains an empty embedding cache`() =
        runTest {
            val app = RuntimeEnvironment.getApplication()
            val file = app.getDatabasePath("migration-v3.db")
            file.parentFile?.mkdirs()
            file.delete()

            SQLiteDatabase.openOrCreateDatabase(file, null).use { legacy ->
                createFromExport(legacy, version = 3)
                legacy.execSQL(
                    "INSERT INTO labels (serverId, name, color, description, isFavourite, isPrivate, createdAt, " +
                        "updatedAt) VALUES (7, 'bug', 'red', 'Ошибки', 0, 0, 1, 1)",
                )
                legacy.version = 3
            }

            val db =
                Room.databaseBuilder(app, TurboistDatabase::class.java, file.absolutePath)
                    .allowMainThreadQueries()
                    .build()
            try {
                assertEquals("Ошибки", db.labels().byServerId(7)?.description)
                assertEquals(emptyList(), db.textEmbeddings().embeddingKeys())
            } finally {
                db.close()
                file.delete()
            }
        }

    private fun createFromExport(
        target: SQLiteDatabase,
        version: Int,
    ) {
        val export = readExport(version)
        for (entity in export.getValue("entities").jsonArray.map { it.jsonObject }) {
            val table = entity.getValue("tableName").jsonPrimitive.content
            target.execSQL(entity.getValue("createSql").jsonPrimitive.content.replace("\${TABLE_NAME}", table))
            entity["indices"]?.jsonArray?.forEach { index ->
                target.execSQL(
                    index.jsonObject.getValue("createSql").jsonPrimitive.content.replace("\${TABLE_NAME}", table),
                )
            }
            entity["contentSyncTriggers"]?.jsonArray?.forEach { target.execSQL(it.jsonPrimitive.content) }
        }
        export.getValue("setupQueries").jsonArray.forEach { target.execSQL(it.jsonPrimitive.content) }
    }

    private fun readExport(version: Int): JsonObject {
        val file =
            File(System.getProperty("turboist.schemaExport.dir").orEmpty())
                .resolve(TurboistDatabase::class.java.canonicalName!!)
                .resolve("$version.json")
        return Json.parseToJsonElement(file.readText()).jsonObject.getValue("database").jsonObject
    }
}
