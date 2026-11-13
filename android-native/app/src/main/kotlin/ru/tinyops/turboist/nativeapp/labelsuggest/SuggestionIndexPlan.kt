package ru.tinyops.turboist.nativeapp.labelsuggest

import ru.tinyops.turboist.core.database.dao.SuggestionWorkspace
import ru.tinyops.turboist.core.model.labelsuggest.HistoryTask
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionIndex
import ru.tinyops.turboist.core.model.labelsuggest.LabelVector
import ru.tinyops.turboist.core.model.labelsuggest.ProjectText
import ru.tinyops.turboist.core.model.labelsuggest.ProjectVector
import ru.tinyops.turboist.core.model.labelsuggest.SuggestionTexts
import ru.tinyops.turboist.core.model.labelsuggest.TaskLabelFacts
import ru.tinyops.turboist.core.model.labelsuggest.destinationPriors
import ru.tinyops.turboist.core.model.labelsuggest.projectLabelWeights
import java.security.MessageDigest

/**
 * What has to be embedded for one snapshot of the workspace, and how the
 * results become an index.
 *
 * Kept apart from the engine and from the database so the whole of it can be
 * checked with plain values: which text stands for which row, and which rows
 * make it into the index once their vectors exist.
 */
internal class SuggestionIndexPlan(
    workspace: SuggestionWorkspace,
) {
    private val labelsOfTask: Map<Long, Set<Long>> =
        workspace.taskLabels.groupBy({ it.ownerLocalId }, { it.labelLocalId }).mapValues { it.value.toSet() }
    private val labelsOfProject: Map<Long, Set<Long>> =
        workspace.projectLabels.groupBy({ it.ownerLocalId }, { it.labelLocalId }).mapValues { it.value.toSet() }
    private val projectTexts: Map<Long, ProjectText> =
        workspace.projects.associate { it.localId to ProjectText(it.title, it.description) }

    /** Each label's text: its name, description and the projects carrying it. */
    val labelTexts: Map<Long, String> =
        workspace.labels.associate { label ->
            val carriers =
                workspace.projectLabels
                    .filter { it.labelLocalId == label.localId }
                    .mapNotNull { projectTexts[it.ownerLocalId] }
            label.localId to SuggestionTexts.labelDocument(label.name, label.description, carriers)
        }

    val projectDocuments: Map<Long, String> = projectTexts.mapValues { SuggestionTexts.projectDocument(it.value) }

    /** Tasks newest first, so a partial index already knows the most relevant history. */
    private val tasksNewestFirst = workspace.tasks.filter { it.title.isNotBlank() }.sortedByDescending { it.createdAt }

    val taskTexts: List<Pair<Long, String>> = tasksNewestFirst.map { it.localId to SuggestionTexts.query(it.title) }

    private val facts: List<TaskLabelFacts> =
        workspace.tasks.map { TaskLabelFacts(it.localId, it.projectLocalId, labelsOfTask[it.localId].orEmpty()) }
    private val createdAt: Map<Long, Long> = workspace.tasks.associate { it.localId to it.createdAt }

    /** Every text the plan needs, labels and projects first: an index is useless without them. */
    val allTexts: List<String> =
        (labelTexts.values + projectDocuments.values + taskTexts.map { it.second }).distinct()

    /**
     * The index over whatever has a vector so far. A label or project without
     * one is left out entirely; a task without one simply does not vote yet.
     */
    fun build(vectorOf: (String) -> FloatArray?): LabelSuggestionIndex {
        val labels = labelTexts.mapNotNull { (id, text) -> vectorOf(text)?.let { LabelVector(id, it) } }
        val weights = projectLabelWeights(projectDocuments.keys, labelsOfProject, facts)
        val projects =
            projectDocuments.mapNotNull { (id, text) ->
                vectorOf(text)?.let { ProjectVector(id, it, weights[id].orEmpty()) }
            }
        val history =
            taskTexts.mapNotNull { (id, text) ->
                vectorOf(text)?.let { HistoryTask(id, it, labelsOfTask[id].orEmpty(), createdAt.getValue(id)) }
            }
        return LabelSuggestionIndex(labels, projects, history, destinationPriors(labelsOfProject, facts))
    }

    companion object {
        /** The cache key of a text under the current model. */
        fun key(text: String): String {
            val digest = MessageDigest.getInstance("SHA-256").digest("${EmbeddingModel.ID}\n$text".toByteArray())
            return digest.joinToString("") { "%02x".format(it) }
        }
    }
}
