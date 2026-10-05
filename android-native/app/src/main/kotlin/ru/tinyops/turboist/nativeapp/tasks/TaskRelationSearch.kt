package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.TaskDao
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.nativeapp.search.SearchFilters
import ru.tinyops.turboist.nativeapp.search.SearchKind
import ru.tinyops.turboist.nativeapp.search.SearchRepository
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Finding the other end of a link.
 *
 * A narrow port so the picker depends on "which tasks match this" rather than on
 * the replica, and so the screen can be driven in a test with no database.
 */
fun interface TaskRelationSearch {
    /**
     * The tasks worth offering for what the user has typed so far.
     *
     * Text too short to mean anything comes back empty rather than refused:
     * deciding whether to ask is the caller's business, and this has to be safe
     * to call on every keystroke.
     */
    suspend fun candidates(typed: String): List<TaskRelationCandidate>

    companion object {
        /** Offers nothing. For a screen composed without a device behind it. */
        val None: TaskRelationSearch = TaskRelationSearch { emptyList() }
    }
}

/**
 * The picker answered from the device, and from nothing else.
 *
 * Linking two tasks is exactly the moment a phone is least likely to have a
 * connection — it happens while planning, wherever the user happens to be — and
 * every task they could link to is already here. So this reuses the same local
 * index the search screen runs on rather than asking the server who exists.
 *
 * A term that is a plain number is also looked up as the id the server knows a
 * task by, because that is the id printed in the web client's address bar and
 * the one a person copies out of it. The id hit leads: only the user knows
 * whether they meant an id or a word, and the id is the more exact reading.
 */
@Singleton
class ReplicaTaskRelationSearch
    @Inject
    constructor(
        private val search: SearchRepository,
        private val tasks: TaskDao,
        private val projects: ProjectDao,
    ) : TaskRelationSearch {
        override suspend fun candidates(typed: String): List<TaskRelationCandidate> {
            val term = typed.trim()
            if (term.isEmpty()) return emptyList()
            val byId = term.toLongOrNull()?.takeIf { it > 0 }?.let { tasks.byServerId(it) }
            val byText =
                if (term.length < MIN_TERM_LENGTH) {
                    emptyList()
                } else {
                    // Finished work is offered too: a task is often linked to
                    // something that has already happened, and hiding it would
                    // leave the user unable to say so.
                    search.search(term, SearchFilters(kind = SearchKind.TASKS)).tasks.map {
                        TaskRelationCandidate(
                            taskLocalId = it.task.localId,
                            serverId = it.task.serverId,
                            title = it.task.title,
                            status = it.task.status,
                            projectTitle = it.projectTitle,
                        )
                    }
                }
            val idHit =
                byId?.let { row ->
                    val task = row.toDomain()
                    TaskRelationCandidate(
                        taskLocalId = task.localId,
                        serverId = task.serverId,
                        title = task.title,
                        status = task.status,
                        projectTitle = task.projectLocalId?.let { projects.byLocalId(it)?.title },
                    )
                }
            if (idHit == null) return byText
            return listOf(idHit) + byText.filterNot { it.taskLocalId == idHit.taskLocalId }
        }

        private companion object {
            /**
             * How much text is worth searching for.
             *
             * One character matches most of a workspace, which is a list nobody
             * can read. A single digit is the exception and is handled as an id
             * rather than as a word.
             */
            const val MIN_TERM_LENGTH = 2
        }
    }
