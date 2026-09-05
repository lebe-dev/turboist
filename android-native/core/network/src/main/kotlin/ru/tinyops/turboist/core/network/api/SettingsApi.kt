package ru.tinyops.turboist.core.network.api

import kotlinx.serialization.json.JsonObject
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.Header
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.PUT
import ru.tinyops.turboist.core.network.ApiHeaders
import ru.tinyops.turboist.core.network.dto.AppSettingsDto
import ru.tinyops.turboist.core.network.dto.AutoLabelsRequest
import ru.tinyops.turboist.core.network.dto.HarpoonDto
import ru.tinyops.turboist.core.network.dto.HarpoonRefRequest
import ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest
import ru.tinyops.turboist.core.network.dto.ProjectSuggestionsRequest
import ru.tinyops.turboist.core.network.dto.UserSettingsDto

/**
 * The three preference surfaces, which are deliberately not one.
 *
 * The user's own preferences and the installation-wide rules have different
 * owners and different write paths and must never be merged; the interface state
 * is a third thing again — an opaque blob the server merges key by key, so a key
 * this build does not know about survives a write from this build untouched.
 */
interface SettingsApi {
    @GET("api/v1/settings")
    suspend fun userSettings(): UserSettingsDto

    @PATCH("api/v1/settings")
    suspend fun patchUserSettings(
        @Body body: PatchUserSettingsRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<UserSettingsDto>

    @GET("api/v1/app-settings")
    suspend fun appSettings(): AppSettingsDto

    /** Replaces the whole rule list; the rules are applied automatically on create. */
    @PUT("api/v1/app-settings/auto-labels")
    suspend fun putAutoLabels(
        @Body body: AutoLabelsRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<AppSettingsDto>

    /** Replaces the whole rule list; these rules are only ever offered as suggestions, never applied. */
    @PUT("api/v1/app-settings/project-suggestions")
    suspend fun putProjectSuggestions(
        @Body body: ProjectSuggestionsRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<AppSettingsDto>

    /** The interface-state blob, returned verbatim. */
    @GET("api/v1/state")
    suspend fun state(): JsonObject

    /**
     * Shallow-merges the given keys into the stored state and answers with the
     * merged result. A key sent as null is removed.
     */
    @PATCH("api/v1/state")
    suspend fun patchState(
        @Body body: JsonObject,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<JsonObject>

    /** The two-slot jump pair, hydrated with titles for display. */
    @GET("api/v1/harpoon")
    suspend fun harpoon(): HarpoonDto

    /** Attaches an entity to the pair. A third attachment evicts the oldest. */
    @POST("api/v1/harpoon/attach")
    suspend fun attachHarpoon(
        @Body body: HarpoonRefRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<HarpoonDto>

    @POST("api/v1/harpoon/detach")
    suspend fun detachHarpoon(
        @Body body: HarpoonRefRequest,
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<HarpoonDto>
}
