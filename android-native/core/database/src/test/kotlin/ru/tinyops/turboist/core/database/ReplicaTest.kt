package ru.tinyops.turboist.core.database

import androidx.room.Room
import org.junit.After
import org.junit.Before
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.ProjectSectionRow
import ru.tinyops.turboist.core.database.entity.TaskRow

/**
 * A real database, in memory.
 *
 * The replica is a schema, and a schema is only as correct as the engine that
 * enforces it says it is — a stub would let a broken foreign key or a missing
 * trigger pass. So every test in this package runs against SQLite, and asks it
 * the same questions the app will.
 */
@RunWith(RobolectricTestRunner::class)
abstract class ReplicaTest {
    protected lateinit var db: TurboistDatabase

    @Before
    fun openReplica() {
        db =
            Room.inMemoryDatabaseBuilder(RuntimeEnvironment.getApplication(), TurboistDatabase::class.java)
                .build()
    }

    @After
    fun closeReplica() {
        db.close()
    }

    protected fun context(
        name: String = "Work",
        serverId: Long? = null,
    ) = ContextRow(serverId = serverId, name = name, createdAt = NOW, updatedAt = NOW)

    protected fun label(
        name: String = "bug",
        serverId: Long? = null,
    ) = LabelRow(serverId = serverId, name = name, createdAt = NOW, updatedAt = NOW)

    protected fun project(
        contextLocalId: Long,
        title: String = "Website",
        serverId: Long? = null,
    ) = ProjectRow(
        serverId = serverId,
        contextLocalId = contextLocalId,
        title = title,
        createdAt = NOW,
        updatedAt = NOW,
    )

    protected fun section(
        projectLocalId: Long,
        title: String = "Doing",
        serverId: Long? = null,
    ) = ProjectSectionRow(
        serverId = serverId,
        projectLocalId = projectLocalId,
        title = title,
        createdAt = NOW,
        updatedAt = NOW,
    )

    protected fun task(
        title: String = "Write it down",
        serverId: Long? = null,
        contextLocalId: Long? = null,
        projectLocalId: Long? = null,
        sectionLocalId: Long? = null,
        parentLocalId: Long? = null,
        sourceTaskLocalId: Long? = null,
    ) = TaskRow(
        serverId = serverId,
        title = title,
        contextLocalId = contextLocalId,
        projectLocalId = projectLocalId,
        sectionLocalId = sectionLocalId,
        parentLocalId = parentLocalId,
        sourceTaskLocalId = sourceTaskLocalId,
        createdAt = NOW,
        updatedAt = NOW,
    )

    companion object {
        /** 2024-03-09T12:34:56.789Z, the reference instant the wire format is written against. */
        const val NOW: Long = 1_709_987_696_789L
    }
}
