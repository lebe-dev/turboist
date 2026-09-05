package ru.tinyops.turboist.nativeapp.tasks

import android.util.Log
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.api.TaskApi
import ru.tinyops.turboist.core.network.dto.TaskDto
import ru.tinyops.turboist.core.network.mapping.ReplicaIds
import ru.tinyops.turboist.core.network.mapping.toTask
import java.io.IOException
import javax.inject.Inject
import javax.inject.Singleton

/** One answer to a request for history from beyond the replica's window. */
sealed interface OlderCompletionsResult {
    /**
     * A page of finished work.
     *
     * @property nextOffset where the page after this one starts.
     * @property hasMore whether the server has anything past this page at all.
     *   A page that is empty of *older* rows is not the end — the rows it held
     *   may simply all have been ones the device already has.
     */
    data class Page(
        val tasks: List<Task>,
        val nextOffset: Int,
        val hasMore: Boolean,
    ) : OlderCompletionsResult

    /** The server was not reachable. Nothing is pending and the request is worth repeating. */
    data object Offline : OlderCompletionsResult

    /** The server was reached and would not answer. */
    data object Failed : OlderCompletionsResult
}

/**
 * The completion history the server holds, read a page at a time.
 *
 * The device keeps a bounded stretch of finished work; this is how anything
 * beyond it is looked at. Nothing read here is written to the replica — see
 * [CompletedSource.SERVER] — so it lives exactly as long as the screen that asked
 * for it.
 */
interface OlderCompletions {
    suspend fun page(
        offset: Int,
        limit: Int,
    ): OlderCompletionsResult
}

/**
 * [OlderCompletions] answered by the server.
 *
 * Every failure is a value rather than an exception, and the two are told apart
 * because the screen says different things about them: an unreachable server is
 * a statement about the phone, and a refusal is a statement about the request.
 * Neither is a reason to lose the history already on screen, which came from the
 * replica and is unaffected.
 */
@Singleton
class ApiOlderCompletions
    @Inject
    constructor(
        private val api: TaskApi,
        private val projects: ProjectDao,
    ) : OlderCompletions {
        override suspend fun page(
            offset: Int,
            limit: Int,
        ): OlderCompletionsResult =
            try {
                val page = api.completed(days = WIDEST_SERVED_WINDOW_DAYS, limit = limit, offset = offset)
                val ids = projectIds(page.items)
                Log.i(COMPLETED_LOG_TAG, "Read ${page.items.size} completed tasks from the server at offset $offset")
                OlderCompletionsResult.Page(
                    // A list payload carries no relation edges, and an empty list
                    // there means "not loaded" rather than "has none" — so they
                    // are deliberately not built from it.
                    tasks = page.items.map { it.toTask(ids, hydrateRelations = false) },
                    nextOffset = offset + page.items.size,
                    hasMore = page.hasMore,
                )
            } catch (unreachable: ApiException.Network) {
                Log.i(COMPLETED_LOG_TAG, "Older completion history needs a connection", unreachable)
                OlderCompletionsResult.Offline
            } catch (refused: ApiException) {
                Log.w(COMPLETED_LOG_TAG, "The server would not serve older completion history", refused)
                OlderCompletionsResult.Failed
            } catch (transport: IOException) {
                Log.i(COMPLETED_LOG_TAG, "Older completion history did not reach the server", transport)
                OlderCompletionsResult.Offline
            }

        /**
         * The device's own ids for the projects a page mentions, and nothing else.
         *
         * A fetched row is drawn, never stored, so the only reference it needs
         * resolved is the one the row shows: the folder it lives in. Everything
         * else stays unresolved, which is also what keeps a fetched task from
         * being mistaken for one the replica holds — it comes back with no device
         * id at all.
         */
        private suspend fun projectIds(items: List<TaskDto>): ReplicaIds {
            val resolved =
                items
                    .mapNotNull { it.projectId }
                    .distinct()
                    .associateWith { projects.localIdForServerId(it) }
            return object : ReplicaIds {
                override fun task(serverId: Long?): Long? = null

                override fun project(serverId: Long?): Long? = serverId?.let { resolved[it] }

                override fun section(serverId: Long?): Long? = null

                override fun context(serverId: Long?): Long? = null

                override fun label(serverId: Long?): Long? = null

                override fun template(serverId: Long?): Long? = null
            }
        }

        private companion object {
            /**
             * How far back one request reaches.
             *
             * The server serves a bounded stretch of history in a single request
             * and clamps anything larger down to it, so this asks for more than it
             * expects to be given: the point is to be handed the widest window the
             * server has, whatever that turns out to be, rather than to encode the
             * server's current limit here and quietly stop at it.
             */
            const val WIDEST_SERVED_WINDOW_DAYS = 3_650
        }
    }

internal const val COMPLETED_LOG_TAG = "TurboistCompleted"
