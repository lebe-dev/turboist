package ru.tinyops.turboist.core.network.api

import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.Header
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query
import ru.tinyops.turboist.core.network.ApiHeaders
import ru.tinyops.turboist.core.network.dto.BulkIdsRequest
import ru.tinyops.turboist.core.network.dto.BulkMoveRequest
import ru.tinyops.turboist.core.network.dto.BulkPriorityRequest
import ru.tinyops.turboist.core.network.dto.BulkResultDto
import ru.tinyops.turboist.core.network.dto.CompleteTaskRequest
import ru.tinyops.turboist.core.network.dto.CreateTaskRelationRequest
import ru.tinyops.turboist.core.network.dto.CreateTaskRequest
import ru.tinyops.turboist.core.network.dto.DecomposeTaskRequest
import ru.tinyops.turboist.core.network.dto.DecomposeTaskResponse
import ru.tinyops.turboist.core.network.dto.GroupTasksRequest
import ru.tinyops.turboist.core.network.dto.GroupTasksResponse
import ru.tinyops.turboist.core.network.dto.MoveTaskRequest
import ru.tinyops.turboist.core.network.dto.PageDto
import ru.tinyops.turboist.core.network.dto.PatchTaskRequest
import ru.tinyops.turboist.core.network.dto.PlanTaskRequest
import ru.tinyops.turboist.core.network.dto.TaskDto

/**
 * Everything that creates or changes a task, plus the few task reads that cannot
 * be answered from the replica.
 *
 * **Why the writes return `Response<T>`.** These are the calls a queued write
 * replays, and a replay needs to know whether the server executed it or answered
 * from its record of an earlier attempt. That marker is a response header, so the
 * response object has to survive as far as the caller; `requireBody()` unwraps it
 * where the marker is not interesting. Failures never arrive this way — they are
 * already typed exceptions by the time a `Response` exists.
 *
 * **Why every write takes an idempotency key.** A queued write must replay under
 * the key it was stored with, so the key is part of the call rather than
 * something generated behind it. Passing `null` lets a fresh key be minted, which
 * is right for a write made while the app is online and watching.
 */
interface TaskApi {
    /**
     * @param relations ask for the relation edges in the same round-trip.
     * @param subtasks ask for the children in the same round-trip.
     */
    @GET("api/v1/tasks/{id}")
    suspend fun get(
        @Path("id") id: Long,
        @Query("relations") relations: Boolean? = null,
        @Query("subtasks") subtasks: Boolean? = null,
    ): TaskDto

    @GET("api/v1/tasks/{id}/subtasks")
    suspend fun subtasks(
        @Path("id") id: Long,
        @Query("limit") limit: Int? = null,
        @Query("offset") offset: Int? = null,
    ): PageDto<TaskDto>

    /**
     * Completed history. The replica holds a recent window of it; anything older
     * than that window only exists here.
     *
     * @param days how far back the window reaches, today always included. The
     *   server clamps it to the stretch it will serve, so asking for more than
     *   that is answered with that much rather than refused.
     */
    @GET("api/v1/tasks/completed")
    suspend fun completed(
        @Query("days") days: Int? = null,
        @Query("limit") limit: Int? = null,
        @Query("offset") offset: Int? = null,
    ): PageDto<TaskDto>

    @POST("api/v1/inbox/tasks")
    suspend fun createInInbox(
        @Body body: CreateTaskRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/projects/{id}/tasks")
    suspend fun createInProject(
        @Path("id") projectId: Long,
        @Body body: CreateTaskRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/sections/{id}/tasks")
    suspend fun createInSection(
        @Path("id") sectionId: Long,
        @Body body: CreateTaskRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/contexts/{id}/tasks")
    suspend fun createInContext(
        @Path("id") contextId: Long,
        @Body body: CreateTaskRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/tasks/{id}/subtasks")
    suspend fun createSubtask(
        @Path("id") parentId: Long,
        @Body body: CreateTaskRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @PATCH("api/v1/tasks/{id}")
    suspend fun patch(
        @Path("id") id: Long,
        @Body body: PatchTaskRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @DELETE("api/v1/tasks/{id}")
    suspend fun delete(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<Unit>

    /**
     * Ticks a task off. Refused while any task blocking it is still open, and the
     * refusal names the blockers.
     */
    @POST("api/v1/tasks/{id}/complete")
    suspend fun complete(
        @Path("id") id: Long,
        @Body body: CompleteTaskRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/tasks/{id}/uncomplete")
    suspend fun uncomplete(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    /** Closes a task without completing it. Like a completion, it releases whatever it was blocking. */
    @POST("api/v1/tasks/{id}/cancel")
    suspend fun cancel(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/tasks/{id}/pin")
    suspend fun pin(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/tasks/{id}/unpin")
    suspend fun unpin(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/tasks/{id}/move")
    suspend fun move(
        @Path("id") id: Long,
        @Body body: MoveTaskRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/tasks/{id}/plan")
    suspend fun plan(
        @Path("id") id: Long,
        @Body body: PlanTaskRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/tasks/{id}/duplicate")
    suspend fun duplicate(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    /** Replaces a task with several siblings cut from it, inheriting its placement and attributes. */
    @POST("api/v1/tasks/{id}/decompose")
    suspend fun decompose(
        @Path("id") id: Long,
        @Body body: DecomposeTaskRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<DecomposeTaskResponse>

    /** Answers with the task the relation was added to, its relations included. */
    @POST("api/v1/tasks/{id}/relations")
    suspend fun addRelation(
        @Path("id") id: Long,
        @Body body: CreateTaskRelationRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @DELETE("api/v1/tasks/{id}/relations/{relationId}")
    suspend fun removeRelation(
        @Path("id") id: Long,
        @Path("relationId") relationId: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskDto>

    @POST("api/v1/tasks/bulk/complete")
    suspend fun bulkComplete(
        @Body body: BulkIdsRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<BulkResultDto>

    @POST("api/v1/tasks/bulk/move")
    suspend fun bulkMove(
        @Body body: BulkMoveRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<BulkResultDto>

    @POST("api/v1/tasks/bulk/priority")
    suspend fun bulkPriority(
        @Body body: BulkPriorityRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<BulkResultDto>

    @POST("api/v1/tasks/group")
    suspend fun group(
        @Body body: GroupTasksRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<GroupTasksResponse>
}
