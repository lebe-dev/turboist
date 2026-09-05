package ru.tinyops.turboist.nativeapp.e2e

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import ru.tinyops.turboist.core.model.ClientKind
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.ContextDto
import ru.tinyops.turboist.core.network.dto.LoginResponseDto
import ru.tinyops.turboist.core.network.dto.ProjectDto
import ru.tinyops.turboist.core.network.dto.SyncSnapshotDto
import ru.tinyops.turboist.core.network.dto.TaskDto
import java.util.concurrent.TimeUnit

/**
 * The server as the test itself sees it, over a channel the app's radios cannot
 * take away.
 *
 * It exists because half of what an offline test has to do is not offline. The
 * dataset the device is expected to pull has to be put there; a row has to be
 * deleted underneath a device that is holding an unsent change against it; and
 * the only trustworthy check of "did that write land, exactly once" is asking
 * the server rather than the copy on the device, which is the thing being
 * doubted.
 *
 * It signs in as a command-line client on purpose. Sessions are budgeted per
 * client kind, so a checking session that claimed to be the Android app would
 * spend a slot the app under test is entitled to and could eventually evict it.
 *
 * Every call is blocking. It is driven from the test thread, where waiting is
 * the point.
 */
class ServerControl(private val baseUrl: String) {
    private val http =
        OkHttpClient
            .Builder()
            .connectTimeout(HTTP_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .readTimeout(HTTP_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .build()

    private var accessToken: String? = null

    /** Takes a session on the server. The account must already exist. */
    fun signIn(
        username: String,
        password: String,
    ) {
        val body =
            buildJsonObject {
                put("username", username)
                put("password", password)
                put("clientKind", ClientKind.CLI.wire)
            }
        val response = post<LoginResponseDto>("auth/login", body, authenticated = false)
        accessToken =
            response.access?.takeIf { it.isNotBlank() }
                ?: error("the server answered the sign-in without an access token")
    }

    /**
     * A context, with a colour.
     *
     * The colour is not decoration here: the schema accepts a named colour or a
     * hex triple and nothing else, so a row created without one is refused.
     */
    fun createContext(name: String): ContextDto =
        post(
            "api/v1/contexts",
            buildJsonObject {
                put("name", name)
                put("color", COLOR)
            },
        )

    fun createProject(
        contextId: Long,
        title: String,
    ): ProjectDto =
        post(
            "api/v1/contexts/$contextId/projects",
            buildJsonObject {
                put("title", title)
                put("color", COLOR)
            },
        )

    fun createProjectTask(
        projectId: Long,
        title: String,
    ): TaskDto = post("api/v1/projects/$projectId/tasks", buildJsonObject { put("title", title) })

    fun createInboxTask(title: String): TaskDto = post("api/v1/inbox/tasks", buildJsonObject { put("title", title) })

    /** Removes a task the way another client would: a hard delete, no tombstone left behind. */
    fun deleteTask(taskId: Long) {
        send(request("api/v1/tasks/$taskId").delete().build())
    }

    /** Every syncable row the server holds — the one read that can prove an absence. */
    fun snapshot(): SyncSnapshotDto = decode(send(request("api/v1/sync/snapshot").get().build()))

    /** The tasks the server holds under this exact title, which is how a duplicate shows up. */
    fun tasksTitled(title: String): List<TaskDto> = snapshot().tasks.filter { it.title == title }

    private inline fun <reified T> post(
        path: String,
        body: JsonObject,
        authenticated: Boolean = true,
    ): T = decode(send(request(path, authenticated).post(body.asRequestBody()).build()))

    private fun request(
        path: String,
        authenticated: Boolean = true,
    ): Request.Builder {
        val builder = Request.Builder().url(baseUrl.trimEnd('/') + "/" + path.trimStart('/'))
        if (!authenticated) return builder
        val token = accessToken ?: error("the checking client has not signed in yet")
        return builder.header("Authorization", "Bearer $token")
    }

    private fun send(request: Request): String =
        http.newCall(request).execute().use { response ->
            val body = response.body?.string().orEmpty()
            check(response.isSuccessful) {
                "${request.method} ${request.url.encodedPath} answered ${response.code}: $body"
            }
            body
        }

    private inline fun <reified T> decode(body: String): T = TurboistJson.decodeFromString(body)

    private fun JsonObject.asRequestBody(): RequestBody =
        TurboistJson.encodeToString(JsonObject.serializer(), this).toRequestBody(JSON)

    private companion object {
        const val HTTP_TIMEOUT_SECONDS: Long = 30

        /** One of the named colours the schema accepts. Which one is of no consequence. */
        const val COLOR: String = "blue"

        val JSON = "application/json; charset=utf-8".toMediaType()
    }
}
