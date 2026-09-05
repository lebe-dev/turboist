package ru.tinyops.turboist.core.network.api

import kotlinx.serialization.json.JsonObject
import retrofit2.Response
import retrofit2.http.Header
import retrofit2.http.POST
import ru.tinyops.turboist.core.network.ApiHeaders

/**
 * The two controls of the daily plan's session.
 *
 * Both answer with the refreshed view of the three slots, and the answer is
 * deliberately left as raw JSON: the slots are counters the server keeps and the
 * replica does not carry, so the view is read back from the next catch-up like
 * everything else. Modelling it here would be a second definition of state
 * nothing on this side owns.
 *
 * As with every other write, the key is a parameter rather than something
 * generated behind the call, because a queued write has to replay under the key
 * it was stored with.
 */
interface TroikiApi {
    /** Begins a cycle, which hands the slots their capacities for the day. */
    @POST("api/v1/troiki/start")
    suspend fun start(
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<JsonObject>

    /** Ends a cycle and empties every slot. */
    @POST("api/v1/troiki/reset")
    suspend fun reset(
        @Header(ApiHeaders.IDEMPOTENCY_KEY) idempotencyKey: String? = null,
    ): Response<JsonObject>
}
