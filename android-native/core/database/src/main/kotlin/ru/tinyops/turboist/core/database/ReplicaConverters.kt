package ru.tinyops.turboist.core.database

import androidx.room.TypeConverter
import ru.tinyops.turboist.core.database.sync.OutboxState
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory

/**
 * How the replica stores the enumerated columns.
 *
 * Every domain enum is written as the spelling the server uses, never as its
 * Kotlin name and never as an ordinal. Two reasons, both about surviving change:
 * a stored ordinal turns into a different value the moment a constant is
 * inserted into the middle of an enum, and a stored server spelling means a row
 * dumped from the device reads the same as the same row dumped from the server.
 *
 * Reading is tolerant in the same way the payload decoders are: a value this
 * build has never heard of becomes the enum's unknown sentinel instead of
 * failing the query. A device that has synced under a newer server must still be
 * able to open its own database after being downgraded — losing one attribute of
 * one row is a far smaller failure than a replica that will not load.
 */
class ReplicaConverters {
    @TypeConverter
    fun fromPriority(value: Priority): String = value.wire

    @TypeConverter
    fun toPriority(value: String): Priority = Priority.fromWire(value)

    @TypeConverter
    fun fromTaskStatus(value: TaskStatus): String = value.wire

    @TypeConverter
    fun toTaskStatus(value: String): TaskStatus = TaskStatus.fromWire(value)

    @TypeConverter
    fun fromProjectStatus(value: ProjectStatus): String = value.wire

    @TypeConverter
    fun toProjectStatus(value: String): ProjectStatus = ProjectStatus.fromWire(value)

    @TypeConverter
    fun fromProjectType(value: ProjectType): String = value.wire

    @TypeConverter
    fun toProjectType(value: String): ProjectType = ProjectType.fromWire(value)

    @TypeConverter
    fun fromPlanState(value: PlanState): String = value.wire

    @TypeConverter
    fun toPlanState(value: String): PlanState = PlanState.fromWire(value)

    @TypeConverter
    fun fromDayPart(value: DayPart): String = value.wire

    @TypeConverter
    fun toDayPart(value: String): DayPart = DayPart.fromWire(value)

    @TypeConverter
    fun fromRelationType(value: RelationType): String = value.wire

    @TypeConverter
    fun toRelationType(value: String): RelationType = RelationType.fromWire(value)

    // A task or project outside the daily plan has no bucket at all, which is a
    // different statement from "a bucket this build does not recognise" — hence
    // a nullable column rather than the sentinel.
    @TypeConverter
    fun fromTroikiCategory(value: TroikiCategory?): String? = value?.wire

    @TypeConverter
    fun toTroikiCategory(value: String?): TroikiCategory? = value?.let(TroikiCategory::fromWire)

    @TypeConverter
    fun fromOutboxState(value: OutboxState): String = value.stored

    @TypeConverter
    fun toOutboxState(value: String): OutboxState = OutboxState.fromStored(value)

    @TypeConverter
    fun fromReplicaEntityKind(value: ReplicaEntityKind): String = value.stored

    // Unlike the domain enums this one has no sentinel: a bookkeeping row naming
    // a table that does not exist cannot be acted on, and the only way to notice
    // that is to refuse to read it rather than to invent a kind.
    @TypeConverter
    fun toReplicaEntityKind(value: String): ReplicaEntityKind =
        ReplicaEntityKind.fromStored(value)
            ?: throw IllegalArgumentException("stored row names an unknown entity kind")
}
