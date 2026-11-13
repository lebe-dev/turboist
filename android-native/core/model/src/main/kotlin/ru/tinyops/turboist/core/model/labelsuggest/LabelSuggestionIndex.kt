package ru.tinyops.turboist.core.model.labelsuggest

/**
 * Everything a label or project suggestion is scored against, already embedded.
 *
 * Holds only open projects and the tasks outside closed ones: whoever builds it
 * leaves an archived, completed or cancelled project out altogether.
 *
 * Built once from the replica and reused for every keystroke: only the title
 * being typed has to go through the model while the user waits.
 *
 * Ids are the replica's local ids throughout.
 *
 * @property labels every label, as one vector each.
 * @property projects every project, with the labels it stands for.
 * @property history earlier tasks with what they were labelled.
 * @property destinations per project, how often its tasks carry each label
 *   beyond the project's own — see [destinationPriors].
 * @property centroids per project, what its latest tasks read like on
 *   average — see [projectCentroids].
 * @property projectHistory the earlier tasks that sit in a project, with every
 *   run of a repeating task counted once: what project suggestions ask. A daily
 *   task completed fifty times is fifty rows of one title, which would otherwise
 *   fill every neighbour slot and outvote a single task that matches exactly.
 */
class LabelSuggestionIndex(
    val labels: List<LabelVector>,
    val projects: List<ProjectVector>,
    val history: List<HistoryTask>,
    val destinations: Map<Long, Map<Long, Float>> = emptyMap(),
    val centroids: Map<Long, FloatArray> = emptyMap(),
    val projectHistory: List<HistoryTask> = history.filter { it.projectId != null },
) {
    /** True when there is nothing a suggestion could name. */
    val isEmpty: Boolean get() = labels.isEmpty()

    /** The projects in the index, which are exactly the open ones. */
    val projectIds: Set<Long> by lazy { projects.mapTo(HashSet()) { it.projectId } }

    companion object {
        val EMPTY: LabelSuggestionIndex = LabelSuggestionIndex(emptyList(), emptyList(), emptyList())
    }
}

class LabelVector(
    val labelId: Long,
    val vector: FloatArray,
)

/**
 * A project as a voter: when a title reads like the project, the project's
 * labels are offered. [labelWeights] is what [projectLabelWeights] works out.
 */
class ProjectVector(
    val projectId: Long,
    val vector: FloatArray,
    val labelWeights: Map<Long, Float>,
)

/** @property projectId where the task sits; `null` for the inbox. */
class HistoryTask(
    val taskId: Long,
    val vector: FloatArray,
    val labels: Set<Long>,
    val createdAt: Long,
    val projectId: Long? = null,
)

/** The facts about a task the index is built from, without its text. */
data class TaskLabelFacts(
    val taskId: Long,
    val projectId: Long?,
    val labels: Set<Long>,
)

/**
 * The labels each project stands for: its own, at full weight, and every label
 * carried by at least [LabelSuggestionRecipe.PROJECT_FREQUENT_SHARE] of its
 * tasks, at the share that carries it. A project with no labels and no tasks
 * still votes — for nothing — which is what keeps a title that reads like it
 * from lending its weight to some other project's labels.
 */
fun projectLabelWeights(
    projectIds: Collection<Long>,
    projectLabels: Map<Long, Set<Long>>,
    tasks: Collection<TaskLabelFacts>,
): Map<Long, Map<Long, Float>> {
    val byProject = tasks.filter { it.projectId != null }.groupBy { it.projectId!! }
    return projectIds.associateWith { project ->
        val weights = HashMap<Long, Float>()
        val own = projectLabels[project].orEmpty()
        val members = byProject[project].orEmpty()
        if (members.isNotEmpty()) {
            members.flatMap { it.labels }.groupingBy { it }.eachCount().forEach { (label, count) ->
                val share = count.toFloat() / members.size
                if (share >= LabelSuggestionRecipe.PROJECT_FREQUENT_SHARE) weights[label] = share
            }
        }
        for (label in own) weights[label] = 1f
        weights
    }
}

/**
 * Per project, the share of its tasks that carry each label the project does
 * not already give them, shrunk toward nothing by
 * [LabelSuggestionRecipe.DESTINATION_SHRINK] tasks' worth of doubt.
 *
 * The project's own labels are left out because a task filed into the project
 * already has them: they are never what a suggestion is for.
 */
fun destinationPriors(
    projectLabels: Map<Long, Set<Long>>,
    tasks: Collection<TaskLabelFacts>,
): Map<Long, Map<Long, Float>> =
    tasks.filter { it.projectId != null }.groupBy { it.projectId!! }.mapValues { (project, members) ->
        val own = projectLabels[project].orEmpty()
        val denominator = (members.size + LabelSuggestionRecipe.DESTINATION_SHRINK).toFloat()
        members.flatMap { it.labels - own }.groupingBy { it }.eachCount()
            .mapValues { (_, count) -> count / denominator }
    }

/**
 * Per project, the average of its [perProject] newest tasks' vectors, scaled
 * back to unit length: one vector that says what the project's recent work is
 * about. A project's newest tasks rather than all of them, because what a
 * project is used for drifts; a project with no tasks has no centroid.
 */
fun projectCentroids(
    history: Collection<HistoryTask>,
    perProject: Int = ProjectSuggestionRecipe.CENTROID_TASKS,
): Map<Long, FloatArray> =
    history.filter { it.projectId != null }.groupBy { it.projectId!! }.mapValues { (_, members) ->
        val newest = members.sortedByDescending { it.createdAt }.take(perProject)
        val sum = FloatArray(newest.first().vector.size)
        for (task in newest) {
            for (i in sum.indices) sum[i] += task.vector.getOrElse(i) { 0f }
        }
        Vectors.prepare(sum, dimensions = sum.size)
    }
