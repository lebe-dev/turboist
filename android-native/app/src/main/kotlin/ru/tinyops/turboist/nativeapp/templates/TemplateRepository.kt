package ru.tinyops.turboist.nativeapp.templates

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.database.dao.LabelDao
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.TaskTemplateDao
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.TaskTemplate
import javax.inject.Inject
import javax.inject.Singleton

/**
 * What the templates screen reads.
 *
 * Like every other list in the app this is a standing query over the replica
 * rather than a fetch: a template written on another device, or the checklist
 * edited on this one, reaches the screen because the rows behind it changed.
 * Nothing here can fail for want of a connection.
 *
 * The whole set is read at once — templates, their lines, and the label edges of
 * both — because that is what the screen shows, and a workspace holds a handful
 * of templates rather than a table worth paging through.
 */
@Singleton
class TemplateRepository
    @Inject
    constructor(
        private val templates: TaskTemplateDao,
        private val labels: LabelDao,
        private val projects: ProjectDao,
    ) {
        /** Every template, in the order they are offered in, with their lines and labels. */
        fun observeTemplates(): Flow<List<TaskTemplate>> =
            combine(
                templates.observeAll(),
                templates.observeAllSubtasks(),
                templates.observeAllLabels(),
                templates.observeAllSubtaskLabels(),
                labels.observeAll(),
            ) { templateRows, subtaskRows, templateLabels, subtaskLabels, labelRows ->
                val byLocalId = labelRows.associate { it.localId to it.toDomain() }
                val linesByTemplate = subtaskRows.groupBy { it.templateLocalId }
                val labelsByTemplate = templateLabels.groupBy({ it.templateLocalId }, { it.labelLocalId })
                val labelsByLine = subtaskLabels.groupBy({ it.subtaskLocalId }, { it.labelLocalId })
                templateRows.map { row ->
                    row.toDomain().copy(
                        labels = named(labelsByTemplate[row.localId], byLocalId),
                        subtasks =
                            linesByTemplate[row.localId].orEmpty().map { line ->
                                line.toDomain().copy(labels = named(labelsByLine[line.localId], byLocalId))
                            },
                    )
                }
            }

        /** The projects a template can be dropped into. */
        fun observeProjects(): Flow<List<Project>> = projects.observeAll().map { rows -> rows.map { it.toDomain() } }

        /** The labels a template can name. */
        fun observeLabels(): Flow<List<Label>> = labels.observeAll().map { rows -> rows.map { it.toDomain() } }

        /**
         * A label this device has never seen contributes nothing rather than an
         * empty chip: the edge is real, the row it points at is simply not here
         * yet, and the next catch-up brings it.
         */
        private fun named(
            labelLocalIds: List<Long>?,
            known: Map<Long, Label>,
        ): List<Label> = labelLocalIds.orEmpty().mapNotNull { known[it] }
    }
