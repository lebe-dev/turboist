package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Query
import androidx.room.Transaction
import ru.tinyops.turboist.core.model.ProjectStatus

/** A task as suggestions read it: its text, where it is, and when it was made. */
data class SuggestionTaskFacts(
    val localId: Long,
    val title: String,
    val projectLocalId: Long?,
    val createdAt: Long,
)

data class SuggestionLabelFacts(
    val localId: Long,
    val name: String,
    val description: String,
)

data class SuggestionProjectFacts(
    val localId: Long,
    val title: String,
    val description: String,
    /** Only an open project is offered as a place for new work. */
    val status: ProjectStatus = ProjectStatus.OPEN,
)

/** One task-and-label or project-and-label pair, by local ids. */
data class LabelPair(
    val ownerLocalId: Long,
    val labelLocalId: Long,
)

/** Everything a label or project suggestion is worked out from, read in one transaction. */
data class SuggestionWorkspace(
    val tasks: List<SuggestionTaskFacts>,
    val taskLabels: List<LabelPair>,
    val labels: List<SuggestionLabelFacts>,
    val projects: List<SuggestionProjectFacts>,
    val projectLabels: List<LabelPair>,
)

/**
 * What label and project suggestions read from the replica.
 *
 * The workspace is read whole and in narrow columns: suggestions compare a
 * title against every task on the device, and reading descriptions, dates and
 * flags they never look at would only make that slower.
 */
@Dao
interface LabelSuggestionDao {
    @Transaction
    suspend fun workspace(): SuggestionWorkspace =
        SuggestionWorkspace(
            tasks = tasks(),
            taskLabels = taskLabels(),
            labels = labels(),
            projects = projects(),
            projectLabels = projectLabels(),
        )

    @Query("SELECT localId, title, projectLocalId, createdAt FROM tasks")
    suspend fun tasks(): List<SuggestionTaskFacts>

    @Query("SELECT taskLocalId AS ownerLocalId, labelLocalId FROM task_labels")
    suspend fun taskLabels(): List<LabelPair>

    @Query("SELECT localId, name, description FROM labels")
    suspend fun labels(): List<SuggestionLabelFacts>

    @Query("SELECT localId, title, description, status FROM projects")
    suspend fun projects(): List<SuggestionProjectFacts>

    @Query("SELECT projectLocalId AS ownerLocalId, labelLocalId FROM project_labels")
    suspend fun projectLabels(): List<LabelPair>
}
