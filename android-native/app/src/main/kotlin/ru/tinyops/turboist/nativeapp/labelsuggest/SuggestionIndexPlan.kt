package ru.tinyops.turboist.nativeapp.labelsuggest

import ru.tinyops.turboist.core.database.dao.SuggestionWorkspace
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.labelsuggest.HistoryTask
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionIndex
import ru.tinyops.turboist.core.model.labelsuggest.LabelVector
import ru.tinyops.turboist.core.model.labelsuggest.ProjectText
import ru.tinyops.turboist.core.model.labelsuggest.ProjectVector
import ru.tinyops.turboist.core.model.labelsuggest.SuggestionTexts
import ru.tinyops.turboist.core.model.labelsuggest.TaskLabelFacts
import ru.tinyops.turboist.core.model.labelsuggest.destinationPriors
import ru.tinyops.turboist.core.model.labelsuggest.projectCentroids
import ru.tinyops.turboist.core.model.labelsuggest.projectLabelWeights
import java.security.MessageDigest

/**
 * What has to be embedded for one snapshot of the workspace, and how the
 * results become an index.
 *
 * Kept apart from the engine and from the database so the whole of it can be
 * checked with plain values: which text stands for which row, and which rows
 * make it into the index once their vectors exist.
 *
 * Only open projects exist as far as suggestions are concerned. An archived,
 * completed or cancelled project is not a place to suggest, does not vote for
 * labels, does not describe the labels it carries, and its tasks are not
 * history: what it was for is over. Measured on a real workspace, leaving them
 * out cost label suggestions nothing; project suggestions keep their precision
 * at the threshold in `ProjectSuggestionRecipe`.
 */
internal class SuggestionIndexPlan(
    workspace: SuggestionWorkspace,
) {
    private val openProjects = workspace.projects.filter { it.status == ProjectStatus.OPEN }
    private val openProjectIds: Set<Long> = openProjects.mapTo(HashSet()) { it.localId }

    /** The inbox's tasks and the open projects' tasks. */
    private val tasks = workspace.tasks.filter { it.projectLocalId == null || it.projectLocalId in openProjectIds }
    private val projectLabelPairs = workspace.projectLabels.filter { it.ownerLocalId in openProjectIds }

    private val labelsOfTask: Map<Long, Set<Long>> =
        workspace.taskLabels.groupBy({ it.ownerLocalId }, { it.labelLocalId }).mapValues { it.value.toSet() }
    private val labelsOfProject: Map<Long, Set<Long>> =
        projectLabelPairs.groupBy({ it.ownerLocalId }, { it.labelLocalId }).mapValues { it.value.toSet() }
    private val projectTexts: Map<Long, ProjectText> =
        openProjects.associate { it.localId to ProjectText(it.title, it.description) }

    /** Each label's text: its name, description and the projects carrying it. */
    val labelTexts: Map<Long, String> =
        workspace.labels.associate { label ->
            val carriers =
                projectLabelPairs
                    .filter { it.labelLocalId == label.localId }
                    .mapNotNull { projectTexts[it.ownerLocalId] }
            label.localId to SuggestionTexts.labelDocument(label.name, label.description, carriers)
        }

    val projectDocuments: Map<Long, String> = projectTexts.mapValues { SuggestionTexts.projectDocument(it.value) }

    /** Tasks newest first, so a partial index already knows the most relevant history. */
    private val tasksNewestFirst = tasks.filter { it.title.isNotBlank() }.sortedByDescending { it.createdAt }

    val taskTexts: List<Pair<Long, String>> = tasksNewestFirst.map { it.localId to SuggestionTexts.query(it.title) }
    private val textOfTask: Map<Long, String> = taskTexts.toMap()

    private val facts: List<TaskLabelFacts> =
        tasks.map { TaskLabelFacts(it.localId, it.projectLocalId, labelsOfTask[it.localId].orEmpty()) }
    private val createdAt: Map<Long, Long> = tasks.associate { it.localId to it.createdAt }
    private val projectOfTask: Map<Long, Long?> = tasks.associate { it.localId to it.projectLocalId }

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
                vectorOf(text)?.let {
                    HistoryTask(id, it, labelsOfTask[id].orEmpty(), createdAt.getValue(id), projectOfTask[id])
                }
            }
        val runs = onceEach(history)
        return LabelSuggestionIndex(
            labels = labels,
            projects = projects,
            history = history,
            destinations = destinationPriors(labelsOfProject, facts),
            centroids = projectCentroids(runs),
            projectHistory = runs,
        )
    }

    /**
     * The tasks in a project with each title counted once per project, the
     * newest row standing for the rest: the runs of a repeating task are one
     * piece of evidence about where such work goes, not one per completion.
     * Labels keep every run, which measured better — a repeating task's labels
     * are as settled as anything in the history.
     */
    private fun onceEach(history: List<HistoryTask>): List<HistoryTask> {
        val seen = HashSet<Pair<String, Long>>()
        return history
            .filter { it.projectId != null }
            .sortedByDescending { it.createdAt }
            .filter { seen.add(textOfTask.getValue(it.taskId).lowercase() to it.projectId!!) }
    }

    companion object {
        /** The cache key of a text under the current model. */
        fun key(text: String): String {
            val digest = MessageDigest.getInstance("SHA-256").digest("${EmbeddingModel.ID}\n$text".toByteArray())
            return digest.joinToString("") { "%02x".format(it) }
        }
    }
}
