package ru.tinyops.turboist.nativeapp.labels

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.database.dao.LabelDao
import ru.tinyops.turboist.core.database.dao.LabelTaggingFacts
import ru.tinyops.turboist.core.database.dao.LabelUsageDao
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.view.LabelTagging
import ru.tinyops.turboist.core.model.view.LabelUsage
import ru.tinyops.turboist.core.model.view.labelUsage
import ru.tinyops.turboist.nativeapp.tasks.TaskListRepository
import java.time.Instant
import java.time.ZoneId
import javax.inject.Inject
import javax.inject.Singleton

/**
 * What the label screens read.
 *
 * The usage report is worked out here, from the replica, rather than fetched
 * from the server. That is the whole point of it on a phone: the numbers are
 * there in a tunnel, they move the instant a task is tagged or ticked off on
 * this device, and switching the period on screen costs nothing because all
 * three windows are counted in the same pass.
 *
 * The counting runs away from the thread that draws. It walks every tagging in
 * the workspace and it runs again on every write the replica takes, including
 * each page a sync applies — left on the drawing thread that is a dropped frame
 * whenever anything changes at all.
 */
@Singleton
class LabelsRepository
    @Inject
    constructor(
        private val labels: LabelDao,
        private val usage: LabelUsageDao,
        private val tasks: TaskListRepository,
    ) {
        /** Every label in the workspace, by name. */
        fun observeLabels(): Flow<List<Label>> = labels.observeAll().map { rows -> rows.map { it.toDomain() } }

        /** One label, or null once it is gone. */
        fun observeLabel(labelLocalId: Long): Flow<Label?> =
            labels.observeByLocalId(labelLocalId).map { it?.toDomain() }

        /** Every task carrying one label, whatever its status. */
        fun observeTasks(labelLocalId: Long): Flow<List<Task>> = tasks.observeLabel(labelLocalId)

        /**
         * The usage report as of [today].
         *
         * [today] is the day the windows are cut against, and it arrives from the
         * caller rather than from a clock read in here, so a screen left open over
         * midnight is handed the next day's report instead of yesterday's.
         */
        fun observeUsage(
            today: Flow<Instant>,
            zone: ZoneId,
        ): Flow<List<LabelUsage>> =
            combine(observeLabels(), usage.observeTaggings(), today) { known, taggings, at ->
                labelUsage(known, taggings.map { it.toTagging() }, at, zone)
            }.flowOn(Dispatchers.Default)
    }

/** The stored facts, as the counting rules read them. */
private fun LabelTaggingFacts.toTagging(): LabelTagging =
    LabelTagging(
        labelLocalId = labelLocalId,
        taggedAt = taggedAt,
        status = status,
        dueAt = dueAt,
        completedAt = completedAt,
        projectLocalId = projectLocalId,
    )
