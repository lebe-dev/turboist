package ru.tinyops.turboist.core.network.api

import retrofit2.http.GET
import retrofit2.http.Query
import ru.tinyops.turboist.core.network.dto.SyncChangesDto
import ru.tinyops.turboist.core.network.dto.SyncSnapshotDto

/**
 * The two reads a replica is built and kept current from.
 *
 * Every other read the app does is a local query. These are the only calls that
 * bring server state in, which is why they are worth having as their own
 * interface rather than scattered among the entity endpoints.
 */
interface SyncApi {
    /**
     * One page of changes after [since], newest history last.
     *
     * The caller applies a page and stores [SyncChangesDto.cursor] with it, then
     * asks again from that cursor until the server says there is no more. Applying
     * is idempotent — each change is the row's current state, or a tombstone — so
     * a page replayed after a crash costs nothing but the work of writing it twice.
     *
     * Two refusals mean "you cannot catch up from here, take a snapshot": the
     * history was replaced (a restore), or the changes still needed were pruned.
     * Both arrive as their own exception subclasses.
     *
     * @param epoch the history the cursor belongs to. Omitted on the very first
     *   pull after a snapshot only because the snapshot has just supplied it.
     * @param limit page size, capped by the server.
     */
    @GET("api/v1/sync/changes")
    suspend fun changes(
        @Query("since") since: Long,
        @Query("epoch") epoch: Long? = null,
        @Query("limit") limit: Int? = null,
    ): SyncChangesDto

    /**
     * Every syncable row in one payload, plus the position to continue the delta
     * feed from. Taken on first launch and whenever the delta feed refuses to
     * resume.
     *
     * Completed tasks are windowed: the payload carries open tasks plus recent
     * completions, and older history stays online-only.
     */
    @GET("api/v1/sync/snapshot")
    suspend fun snapshot(): SyncSnapshotDto
}
