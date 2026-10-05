package ru.tinyops.turboist.core.database.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.ForeignKey
import androidx.room.Index
import androidx.room.PrimaryKey
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.TroikiCategory

/**
 * The workspace tree: a context holds projects, a project holds sections.
 *
 * Column names are the field names, which are also the names the API's payloads
 * use, so a mapper reads as a straight assignment. Table names are the server's,
 * so the replica and the migrations can be compared side by side.
 *
 * Timestamps are epoch milliseconds throughout the replica — they sort, compare
 * and index without being parsed, which is what a list query wants.
 */
@Entity(
    tableName = "contexts",
    indices = [Index(value = ["serverId"], unique = true)],
)
data class ContextRow(
    @PrimaryKey(autoGenerate = true) override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val name: String,
    val color: String = "",
    val isFavourite: Boolean = false,
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaRow<ContextRow> {
    override fun withLocalId(localId: Long): ContextRow = copy(localId = localId)
}

@Entity(
    tableName = "labels",
    indices = [Index(value = ["serverId"], unique = true)],
)
data class LabelRow(
    @PrimaryKey(autoGenerate = true) override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val name: String,
    val color: String = "",
    val isFavourite: Boolean = false,
    val isPrivate: Boolean = false,
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaRow<LabelRow> {
    override fun withLocalId(localId: Long): LabelRow = copy(localId = localId)
}

/**
 * A project. Deleting its context takes it with it, exactly as the server does —
 * there are no tombstones on either side, so a cascade is a cascade.
 */
@Entity(
    tableName = "projects",
    indices = [
        Index(value = ["serverId"], unique = true),
        Index(value = ["contextLocalId"]),
    ],
    foreignKeys = [
        ForeignKey(
            entity = ContextRow::class,
            parentColumns = ["localId"],
            childColumns = ["contextLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
)
data class ProjectRow(
    @PrimaryKey(autoGenerate = true) override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val contextLocalId: Long,
    val title: String,
    val description: String = "",
    val color: String = "",
    val status: ProjectStatus = ProjectStatus.OPEN,
    val type: ProjectType = ProjectType.GENERIC,
    val isPinned: Boolean = false,
    val pinnedAt: Long? = null,
    val isPrivate: Boolean = false,
    val troikiCategory: TroikiCategory? = null,
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaRow<ProjectRow> {
    override fun withLocalId(localId: Long): ProjectRow = copy(localId = localId)
}

/** A column of a project board. [position] is the left-to-right order. */
@Entity(
    tableName = "project_sections",
    indices = [
        Index(value = ["serverId"], unique = true),
        Index(value = ["projectLocalId", "position"]),
    ],
    foreignKeys = [
        ForeignKey(
            entity = ProjectRow::class,
            parentColumns = ["localId"],
            childColumns = ["projectLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
)
data class ProjectSectionRow(
    @PrimaryKey(autoGenerate = true) override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val projectLocalId: Long,
    val title: String,
    val position: Int = 0,
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaRow<ProjectSectionRow> {
    override fun withLocalId(localId: Long): ProjectSectionRow = copy(localId = localId)
}

/**
 * A label attached to a project. A join row of local ids on both ends, so it
 * survives either side being created offline.
 */
@Entity(
    tableName = "project_labels",
    primaryKeys = ["projectLocalId", "labelLocalId"],
    indices = [Index(value = ["labelLocalId"])],
    foreignKeys = [
        ForeignKey(
            entity = ProjectRow::class,
            parentColumns = ["localId"],
            childColumns = ["projectLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
        ForeignKey(
            entity = LabelRow::class,
            parentColumns = ["localId"],
            childColumns = ["labelLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
)
data class ProjectLabelRow(
    @ColumnInfo(name = "projectLocalId") val projectLocalId: Long,
    @ColumnInfo(name = "labelLocalId") val labelLocalId: Long,
)
