package ru.tinyops.turboist.core.network

import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import ru.tinyops.turboist.core.network.api.ApiTokenApi
import ru.tinyops.turboist.core.network.api.AuthApi
import ru.tinyops.turboist.core.network.api.CalendarApi
import ru.tinyops.turboist.core.network.api.ContextApi
import ru.tinyops.turboist.core.network.api.EventsApi
import ru.tinyops.turboist.core.network.api.LabelApi
import ru.tinyops.turboist.core.network.api.PasskeyApi
import ru.tinyops.turboist.core.network.api.ProjectApi
import ru.tinyops.turboist.core.network.api.SessionApi
import ru.tinyops.turboist.core.network.api.SettingsApi
import ru.tinyops.turboist.core.network.api.SyncApi
import ru.tinyops.turboist.core.network.api.TaskApi
import ru.tinyops.turboist.core.network.api.TemplateApi
import ru.tinyops.turboist.core.network.api.TotpApi
import ru.tinyops.turboist.core.network.api.TroikiApi
import ru.tinyops.turboist.core.network.auth.AccessTokenRefresher
import ru.tinyops.turboist.core.network.auth.AccessTokenSource
import ru.tinyops.turboist.core.network.http.AuthInterceptor
import ru.tinyops.turboist.core.network.http.ErrorMappingInterceptor
import ru.tinyops.turboist.core.network.http.IdempotencyInterceptor
import ru.tinyops.turboist.core.network.http.ServerUrlInterceptor
import java.time.Duration
import java.util.UUID

/**
 * The app's one HTTP stack, and the endpoints reachable through it.
 *
 * A single [OkHttpClient] backs every endpoint group so that the connection pool,
 * the DNS cache and — most importantly — the single-flight token refresh are
 * shared. Two clients would mean two refreshes racing, and the server treats a
 * re-used rotated refresh token as theft.
 *
 * The stack is built once, at start-up, before the server address is even known:
 * the address is read per request from [serverUrl], so the connect screen can set
 * it, change it, or clear it without anything being rebuilt.
 */
class TurboistNetwork private constructor(
    val serverUrl: ServerUrl,
    val httpClient: OkHttpClient,
    private val retrofit: Retrofit,
) {
    val auth: AuthApi = retrofit.create(AuthApi::class.java)
    val sync: SyncApi = retrofit.create(SyncApi::class.java)
    val events: EventsApi = retrofit.create(EventsApi::class.java)
    val tasks: TaskApi = retrofit.create(TaskApi::class.java)
    val projects: ProjectApi = retrofit.create(ProjectApi::class.java)
    val contexts: ContextApi = retrofit.create(ContextApi::class.java)
    val labels: LabelApi = retrofit.create(LabelApi::class.java)
    val templates: TemplateApi = retrofit.create(TemplateApi::class.java)
    val settings: SettingsApi = retrofit.create(SettingsApi::class.java)
    val passkeys: PasskeyApi = retrofit.create(PasskeyApi::class.java)
    val troiki: TroikiApi = retrofit.create(TroikiApi::class.java)

    /**
     * The account's administrative surfaces: which sessions are open, which
     * long-lived tokens exist, and whether a second factor is on.
     *
     * They go through the same stack as everything else, and feed nothing into
     * the replica. What they report is only true at the moment it is asked for,
     * so it is read live and dropped again rather than stored.
     */
    val sessions: SessionApi = retrofit.create(SessionApi::class.java)
    val apiTokens: ApiTokenApi = retrofit.create(ApiTokenApi::class.java)
    val totp: TotpApi = retrofit.create(TotpApi::class.java)

    /**
     * The external calendar. Reached through the same stack as everything else
     * so it shares the token refresh, but it feeds nothing into the replica:
     * what it returns is displayed and cached for offline reading, never synced.
     */
    val calendars: CalendarApi = retrofit.create(CalendarApi::class.java)

    companion object {
        /**
         * How long to wait for a connection. Short, because a wrong address or a
         * server that is simply not there should tell the user quickly rather than
         * leave a spinner running.
         */
        private val CONNECT_TIMEOUT: Duration = Duration.ofSeconds(15)

        /**
         * How long to wait for data once a connection exists. Generous on purpose:
         * the replica seed is one large response, and cutting it off on a slow
         * mobile link would leave the app with no data at all.
         */
        private val IO_TIMEOUT: Duration = Duration.ofSeconds(45)

        /**
         * Builds the stack.
         *
         * **Interceptor order is part of the contract**, so it is fixed here rather
         * than left to the caller:
         *
         * 1. Error mapping is outermost, so everything beneath it still sees real
         *    responses — the 401 that the refresh reacts to above all.
         * 2. The address rewrite comes next, so every layer below works with the
         *    URL that will actually be requested.
         * 3. Idempotency keys are stamped before the refresh retry, so a request
         *    retried with a fresh token keeps the key it already had and cannot
         *    execute twice.
         * 4. The bearer token goes on last, closest to the wire, because it is the
         *    only header that may differ between the first attempt and the retry.
         *
         * @param tokens where the access token is read from, per request.
         * @param refresher how an aged-out token is replaced. It must not call back
         *   through an authenticated endpoint of this same stack.
         * @param newIdempotencyKey overridable so a test can assert on a fixed key.
         */
        fun create(
            serverUrl: ServerUrl,
            tokens: AccessTokenSource = AccessTokenSource.None,
            refresher: AccessTokenRefresher = AccessTokenRefresher.None,
            json: Json = TurboistJson,
            newIdempotencyKey: () -> String = { UUID.randomUUID().toString() },
        ): TurboistNetwork {
            val client =
                OkHttpClient.Builder()
                    .connectTimeout(CONNECT_TIMEOUT)
                    .readTimeout(IO_TIMEOUT)
                    .writeTimeout(IO_TIMEOUT)
                    .addInterceptor(ErrorMappingInterceptor())
                    .addInterceptor(ServerUrlInterceptor(serverUrl))
                    .addInterceptor(IdempotencyInterceptor(newIdempotencyKey))
                    .addInterceptor(AuthInterceptor(tokens, refresher))
                    .build()
            val retrofit =
                Retrofit.Builder()
                    // Never contacted: every request is rewritten onto the address
                    // the user configured before it leaves.
                    .baseUrl(ServerUrlInterceptor.PLACEHOLDER_BASE_URL)
                    .client(client)
                    .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
                    .build()
            return TurboistNetwork(serverUrl, client, retrofit)
        }
    }
}
