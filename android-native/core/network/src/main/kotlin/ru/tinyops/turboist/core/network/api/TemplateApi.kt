package ru.tinyops.turboist.core.network.api

import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.Header
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path
import ru.tinyops.turboist.core.network.ApiHeaders
import ru.tinyops.turboist.core.network.dto.InstantiateTemplateRequest
import ru.tinyops.turboist.core.network.dto.InstantiateTemplateResponse
import ru.tinyops.turboist.core.network.dto.TaskTemplateDto
import ru.tinyops.turboist.core.network.dto.TaskTemplateRequest

/**
 * Reusable task blueprints.
 *
 * Editing is a full replace rather than a patch: the editor always submits the
 * whole structure, and a partial update of a nested subtask list has no sane
 * meaning.
 */
interface TemplateApi {
    /** A draft built from an existing task and its flattened descendants, ready to save as a template. */
    @GET("api/v1/tasks/{id}/template-draft")
    suspend fun draftFromTask(
        @Path("id") taskId: Long,
    ): TaskTemplateDto

    @POST("api/v1/task-templates")
    suspend fun create(
        @Body body: TaskTemplateRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskTemplateDto>

    @PATCH("api/v1/task-templates/{id}")
    suspend fun replace(
        @Path("id") id: Long,
        @Body body: TaskTemplateRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<TaskTemplateDto>

    @DELETE("api/v1/task-templates/{id}")
    suspend fun delete(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<Unit>

    /** Creates a task and its subtasks from the template, in the given project. */
    @POST("api/v1/task-templates/{id}/instantiate")
    suspend fun instantiate(
        @Path("id") id: Long,
        @Body body: InstantiateTemplateRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<InstantiateTemplateResponse>
}
