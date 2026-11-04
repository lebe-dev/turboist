package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Query
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.TaskRow

/**
 * How a search result is ordered.
 *
 * The index answers *whether* a row matches and nothing else — it keeps no
 * relevance score of its own — so the order is stated here, as facts about the
 * row rather than as a number nobody can read. Four questions, asked in this
 * order, and each one is a claim about what a person is looking for:
 *
 * 1. **Did the words hit the title?** A task named "passport" is what someone
 *    typing "passport" wants; a task that merely mentions it in its notes is a
 *    second-best answer. This is the one boost worth having, and it is why the
 *    same terms are asked twice — once over every column, once over the title.
 * 2. **Is it still open?** Search is used to get back to work in hand far more
 *    often than to dig through what is finished. Finished work is not hidden —
 *    it is simply behind the work that is not.
 * 3. **How recently was it touched?** Between two equally good matches the one
 *    that moved most recently is the one being worked on.
 * 4. **Which row is it?** The last word, so the order is total and two runs of
 *    the same search cannot come back in different orders.
 *
 * This is deliberately not the server's ordering. The server ranks a substring
 * scan by the shared list sort — pinned first, then priority — because that is
 * the sort every one of its list endpoints uses. On the device the whole
 * workspace is in hand, so the question can be answered as a search question:
 * best match first. The two need not agree, and a device that reproduced the
 * server's ordering would be a worse search for it.
 *
 * A limit is applied after the ordering, so a capped search returns the best
 * matches rather than an arbitrary slice of them.
 */
internal object SearchSql {
    private const val TASK_MATCH = "SELECT rowid FROM tasks_fts WHERE tasks_fts MATCH "

    private const val PROJECT_MATCH = "SELECT rowid FROM projects_fts WHERE projects_fts MATCH "

    const val TASKS =
        "SELECT * FROM tasks WHERE localId IN ($TASK_MATCH:query) " +
            "AND (:status IS NULL OR status = :status) " +
            "ORDER BY (localId IN ($TASK_MATCH:titleQuery)) DESC, " +
            "(status = 'open') DESC, updatedAt DESC, localId DESC LIMIT :limit"

    const val PROJECTS =
        "SELECT * FROM projects WHERE localId IN ($PROJECT_MATCH:query) " +
            "ORDER BY (localId IN ($PROJECT_MATCH:titleQuery)) DESC, " +
            "(status = 'open') DESC, isPinned DESC, updatedAt DESC, localId DESC LIMIT :limit"

    // Labels and contexts carry one indexed column each, so there is no title to
    // prefer over a body: what is left is the user's own mark of importance and
    // then recency.
    const val LABELS =
        "SELECT * FROM labels WHERE localId IN (SELECT rowid FROM labels_fts WHERE labels_fts MATCH :query) " +
            "ORDER BY isFavourite DESC, updatedAt DESC, localId DESC LIMIT :limit"

    private const val LABEL_MATCH = "SELECT rowid FROM labels_fts WHERE labels_fts MATCH :labelQuery"

    private const val TASK_LABEL_HIT =
        "localId IN (SELECT taskLocalId FROM task_labels WHERE labelLocalId IN ($LABEL_MATCH))"

    private const val PROJECT_LABEL_HIT =
        "localId IN (SELECT projectLocalId FROM project_labels WHERE labelLocalId IN ($LABEL_MATCH))"

    // The quick switcher ranks by where the words hit — the title, then a label
    // the row carries, then the description — which is the order the server's
    // search answers in, so the two clients put the same thing first. The rank is
    // returned rather than only applied, because the switcher interleaves tasks
    // and projects by it.
    const val SWITCHER_TASKS =
        "SELECT localId, CASE WHEN localId IN ($TASK_MATCH:titleQuery) THEN 0 " +
            "WHEN $TASK_LABEL_HIT THEN 1 ELSE 2 END AS hitRank FROM tasks " +
            "WHERE localId IN ($TASK_MATCH:query) OR $TASK_LABEL_HIT " +
            "ORDER BY hitRank, (status = 'open') DESC, updatedAt DESC, localId DESC LIMIT :limit"

    const val SWITCHER_PROJECTS =
        "SELECT localId, CASE WHEN localId IN ($PROJECT_MATCH:titleQuery) THEN 0 " +
            "WHEN $PROJECT_LABEL_HIT THEN 1 ELSE 2 END AS hitRank FROM projects " +
            "WHERE localId IN ($PROJECT_MATCH:query) OR $PROJECT_LABEL_HIT " +
            "ORDER BY hitRank, (status = 'open') DESC, isPinned DESC, updatedAt DESC, localId DESC LIMIT :limit"

    const val CONTEXTS =
        "SELECT * FROM contexts WHERE localId IN (SELECT rowid FROM contexts_fts WHERE contexts_fts MATCH :query) " +
            "ORDER BY isFavourite DESC, updatedAt DESC, localId DESC LIMIT :limit"
}

/**
 * One quick-switcher match: the row, and where the words hit it — 0 for the
 * title, 1 for a label it carries, 2 for its description.
 */
data class SwitcherHit(
    val localId: Long,
    val hitRank: Int,
)

/**
 * Search, as four queries against the replica.
 *
 * Built on top of the index rather than inside it: the index says which rows
 * match, and this pairs that answer with the rows themselves and with the order
 * described in [SearchSql]. Nothing here reaches the network — a search is
 * answered entirely from what the device holds, which is why it keeps working,
 * and keeps being fast, with no connection at all.
 *
 * Every `query` argument must already be a valid full-text query; build one with
 * `FtsQuery`. `titleQuery` is the same terms scoped to the title column, and
 * decides the first tiebreak rather than membership — passing a query that
 * matches nothing costs a search its title boost, never its results.
 *
 * These answer once rather than as a standing query. A search is a question
 * asked at a moment, and re-running it under the user because a sync arrived
 * would reorder the list they are reading.
 */
@Dao
interface SearchDao {
    /**
     * Matching tasks, best first.
     *
     * [status] is the wire spelling of a task status, or null for every status —
     * search reaches into finished work as well, which is most of the point of
     * having it.
     */
    @Query(SearchSql.TASKS)
    suspend fun tasks(
        query: String,
        titleQuery: String,
        status: String?,
        limit: Int,
    ): List<TaskRow>

    @Query(SearchSql.PROJECTS)
    suspend fun projects(
        query: String,
        titleQuery: String,
        limit: Int,
    ): List<ProjectRow>

    @Query(SearchSql.LABELS)
    suspend fun labels(
        query: String,
        limit: Int,
    ): List<LabelRow>

    @Query(SearchSql.CONTEXTS)
    suspend fun contexts(
        query: String,
        limit: Int,
    ): List<ContextRow>

    /**
     * Tasks for the quick switcher, best first, with the rank each was found at.
     * `labelQuery` is the terms over the label index, which has one column.
     */
    @Query(SearchSql.SWITCHER_TASKS)
    suspend fun switcherTasks(
        query: String,
        titleQuery: String,
        labelQuery: String,
        limit: Int,
    ): List<SwitcherHit>

    /** Projects for the quick switcher, best first, with the rank each was found at. */
    @Query(SearchSql.SWITCHER_PROJECTS)
    suspend fun switcherProjects(
        query: String,
        titleQuery: String,
        labelQuery: String,
        limit: Int,
    ): List<SwitcherHit>

    /** The task rows named, in no particular order; ids with no row are skipped. */
    @Query("SELECT * FROM tasks WHERE localId IN (:localIds)")
    suspend fun tasksByLocalIds(localIds: List<Long>): List<TaskRow>

    /** The project rows named, in no particular order; ids with no row are skipped. */
    @Query("SELECT * FROM projects WHERE localId IN (:localIds)")
    suspend fun projectsByLocalIds(localIds: List<Long>): List<ProjectRow>
}
