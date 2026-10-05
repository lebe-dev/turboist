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
import ru.tinyops.turboist.core.network.dto.ContextDto
import ru.tinyops.turboist.core.network.dto.CreateContextRequest
import ru.tinyops.turboist.core.network.dto.CreateLabelRequest
import ru.tinyops.turboist.core.network.dto.CreateProjectRequest
import ru.tinyops.turboist.core.network.dto.CreateSectionRequest
import ru.tinyops.turboist.core.network.dto.LabelDto
import ru.tinyops.turboist.core.network.dto.PatchContextRequest
import ru.tinyops.turboist.core.network.dto.PatchLabelRequest
import ru.tinyops.turboist.core.network.dto.PatchProjectRequest
import ru.tinyops.turboist.core.network.dto.PatchSectionRequest
import ru.tinyops.turboist.core.network.dto.ProjectDto
import ru.tinyops.turboist.core.network.dto.ReorderSectionRequest
import ru.tinyops.turboist.core.network.dto.SectionDto
import ru.tinyops.turboist.core.network.dto.SetTroikiCategoryRequest

/**
 * Projects and the sections of their boards.
 *
 * A project is created inside a context, which is why the create call hangs off
 * the context rather than off `/projects`; the same reasoning puts section
 * creation under the project it belongs to. The writes follow the same two rules
 * as the task writes: they answer with a `Response` so a replayed write can be
 * recognised, and they take the idempotency key the caller stored with the write.
 */
interface ProjectApi {
    @GET("api/v1/projects/{id}")
    suspend fun get(
        @Path("id") id: Long,
    ): ProjectDto

    @POST("api/v1/contexts/{id}/projects")
    suspend fun create(
        @Path("id") contextId: Long,
        @Body body: CreateProjectRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ProjectDto>

    @PATCH("api/v1/projects/{id}")
    suspend fun patch(
        @Path("id") id: Long,
        @Body body: PatchProjectRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ProjectDto>

    @DELETE("api/v1/projects/{id}")
    suspend fun delete(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<Unit>

    @POST("api/v1/projects/{id}/complete")
    suspend fun complete(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ProjectDto>

    @POST("api/v1/projects/{id}/uncomplete")
    suspend fun uncomplete(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ProjectDto>

    @POST("api/v1/projects/{id}/cancel")
    suspend fun cancel(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ProjectDto>

    /** Keeps the project readable but moves it out of the way. */
    @POST("api/v1/projects/{id}/archive")
    suspend fun archive(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ProjectDto>

    @POST("api/v1/projects/{id}/unarchive")
    suspend fun unarchive(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ProjectDto>

    @POST("api/v1/projects/{id}/pin")
    suspend fun pin(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ProjectDto>

    @POST("api/v1/projects/{id}/unpin")
    suspend fun unpin(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ProjectDto>

    /**
     * Puts the project in one of the daily slots, or takes it out of all of
     * them. An absent category is what "out of all of them" looks like on the
     * wire, which is why the field is nullable rather than a required enum.
     */
    @POST("api/v1/projects/{id}/troiki")
    suspend fun setTroikiCategory(
        @Path("id") id: Long,
        @Body body: SetTroikiCategoryRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ProjectDto>

    @POST("api/v1/projects/{id}/sections")
    suspend fun createSection(
        @Path("id") projectId: Long,
        @Body body: CreateSectionRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<SectionDto>

    @PATCH("api/v1/sections/{id}")
    suspend fun patchSection(
        @Path("id") id: Long,
        @Body body: PatchSectionRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<SectionDto>

    @DELETE("api/v1/sections/{id}")
    suspend fun deleteSection(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<Unit>

    /** Moves a section along its board. The server owns the final ordering. */
    @POST("api/v1/sections/{id}/reorder")
    suspend fun reorderSection(
        @Path("id") id: Long,
        @Body body: ReorderSectionRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<SectionDto>
}

/** Contexts: the grouping of projects that sits at the top of the workspace tree. */
interface ContextApi {
    @GET("api/v1/contexts/{id}")
    suspend fun get(
        @Path("id") id: Long,
    ): ContextDto

    @POST("api/v1/contexts")
    suspend fun create(
        @Body body: CreateContextRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ContextDto>

    @PATCH("api/v1/contexts/{id}")
    suspend fun patch(
        @Path("id") id: Long,
        @Body body: PatchContextRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<ContextDto>

    @DELETE("api/v1/contexts/{id}")
    suspend fun delete(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<Unit>
}

/** Labels: the tags carried by tasks and projects. */
interface LabelApi {
    @GET("api/v1/labels/{id}")
    suspend fun get(
        @Path("id") id: Long,
    ): LabelDto

    @POST("api/v1/labels")
    suspend fun create(
        @Body body: CreateLabelRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<LabelDto>

    @PATCH("api/v1/labels/{id}")
    suspend fun patch(
        @Path("id") id: Long,
        @Body body: PatchLabelRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<LabelDto>

    @DELETE("api/v1/labels/{id}")
    suspend fun delete(
        @Path("id") id: Long,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<Unit>
}
