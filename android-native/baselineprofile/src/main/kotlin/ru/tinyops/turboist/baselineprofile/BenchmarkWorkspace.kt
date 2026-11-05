package ru.tinyops.turboist.baselineprofile

import androidx.test.platform.app.InstrumentationRegistry
import org.json.JSONArray
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import java.time.Instant
import java.time.temporal.ChronoUnit

/**
 * The workspace every journey runs against: an account on a throwaway server, and
 * enough work in it for the lists to be lists.
 *
 * The screens are what a profile has to cover, and every one of them is behind a
 * sign-in and empty until something is in the replica. It is created over the
 * server's ordinary API rather than planted in the app's database, so the app gets
 * it the way it gets anything: a complete copy on first sign-in.
 *
 * The server address comes from the recipe that started the server, and nothing is
 * defaulted: without one, this would otherwise look for a server that is not there
 * and fail several minutes later for no apparent reason.
 */
internal object BenchmarkWorkspace {
    const val USERNAME: String = "benchmark"
    const val PASSWORD: String = "benchmark-password"

    /**
     * The tasks a journey waits for. A list only holds what is on screen, and it sorts
     * by priority and then newest first, so these are urgent and created last: they
     * top their lists however tall the screen is. The first also sits in the first
     * project, so it tops that board too.
     */
    const val TODAY_ANCHOR: String = "Today anchor"
    const val INBOX_ANCHOR: String = "Inbox anchor"
    const val FIRST_PROJECT: String = "Project 1"

    private const val SERVER_ARGUMENT = "turboistServer"
    private const val CONTEXTS = 2
    private const val PROJECTS_PER_CONTEXT = 3
    private const val TASKS_PER_PROJECT = 30
    private const val TODAY_TASKS = 40
    private const val INBOX_TASKS = 40
    private const val WEEK_TASKS = 40
    private const val COLOR = "blue"
    private val LABELS = listOf("home", "work", "calls", "errands")
    private val PRIORITIES = listOf("high", "medium", "low", "no-priority")

    private const val STATUS_CREATED_BEFORE = 410
    private const val HTTP_OK_FLOOR = 200
    private const val HTTP_OK_CEILING = 299
    private const val TIMEOUT_MILLIS = 30_000

    private const val DESCRIPTION =
        "# Notes\n\nSomething **worth** reading, with a [link](https://example.com).\n\n- one\n- two\n- three\n"

    val serverUrl: String by lazy {
        InstrumentationRegistry.getArguments().getString(SERVER_ARGUMENT)?.trimEnd('/')
            ?: error(
                "This suite drives a real server and was started without \"$SERVER_ARGUMENT\". " +
                    "Run it with `just android-native-baseline-profile` or `just android-native-benchmark`, " +
                    "which start one and pass the address in.",
            )
    }

    /** Creates the account and its work, unless an earlier test in this run already did. */
    fun ensureSeeded() {
        val body =
            JSONObject()
                .put("username", USERNAME)
                .put("password", PASSWORD)
                .put("clientKind", "cli")
        val (status, answer) = send("POST", "auth/setup", body, token = null)
        if (status == STATUS_CREATED_BEFORE) return
        check(status in HTTP_OK_FLOOR..HTTP_OK_CEILING) { "The server refused the account with $status: $answer" }
        seed(JSONObject(answer).getString("access"))
    }

    private fun seed(token: String) {
        val due = Instant.now().truncatedTo(ChronoUnit.SECONDS).toString().replace("Z", ".000Z")
        LABELS.forEach { post(token, "api/v1/labels", JSONObject().put("name", it).put("color", COLOR)) }

        var projectNumber = 0
        val projectIds = mutableListOf<Long>()
        repeat(CONTEXTS) { context ->
            val contextBody = JSONObject().put("name", "Context ${context + 1}").put("color", COLOR)
            val contextId = post(token, "api/v1/contexts", contextBody)
            repeat(PROJECTS_PER_CONTEXT) {
                projectNumber++
                val project = JSONObject().put("title", "Project $projectNumber").put("color", COLOR)
                projectIds += post(token, "api/v1/contexts/$contextId/projects", project)
            }
        }

        projectIds.forEachIndexed { index, projectId ->
            repeat(TASKS_PER_PROJECT) { number ->
                post(token, "api/v1/projects/$projectId/tasks", task("Project ${index + 1} task ${number + 1}", number))
            }
        }
        val home = projectIds.first()
        repeat(TODAY_TASKS) { number ->
            val body = task("Today task %03d".format(number + 1), number).put("dueAt", due)
            post(token, "api/v1/projects/$home/tasks", body)
        }
        repeat(WEEK_TASKS) { number ->
            val body = task("Week task %03d".format(number + 1), number).put("planState", "week")
            post(token, "api/v1/projects/$home/tasks", body)
        }
        repeat(INBOX_TASKS) { number ->
            post(token, "api/v1/inbox/tasks", task("Inbox task %03d".format(number + 1), number))
        }
        post(token, "api/v1/projects/$home/tasks", task(TODAY_ANCHOR, number = 0).put("dueAt", due))
        post(token, "api/v1/inbox/tasks", task(INBOX_ANCHOR, number = 0))
    }

    private fun task(
        title: String,
        number: Int,
    ): JSONObject =
        JSONObject()
            .put("title", title)
            .put("description", DESCRIPTION)
            .put("priority", PRIORITIES[number % PRIORITIES.size])
            .put("labels", JSONArray(listOf(LABELS[number % LABELS.size])))

    /** Creates something and answers with its id. */
    private fun post(
        token: String,
        path: String,
        body: JSONObject,
    ): Long {
        val (status, answer) = send("POST", path, body, token)
        check(status in HTTP_OK_FLOOR..HTTP_OK_CEILING) { "POST $path answered $status: $answer" }
        return JSONObject(answer).getLong("id")
    }

    private fun send(
        method: String,
        path: String,
        body: JSONObject,
        token: String?,
    ): Pair<Int, String> {
        val connection = URL("$serverUrl/$path").openConnection() as HttpURLConnection
        try {
            connection.requestMethod = method
            connection.connectTimeout = TIMEOUT_MILLIS
            connection.readTimeout = TIMEOUT_MILLIS
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/json")
            token?.let { connection.setRequestProperty("Authorization", "Bearer $it") }
            connection.outputStream.use { it.write(body.toString().toByteArray()) }
            val status = connection.responseCode
            val succeeded = status in HTTP_OK_FLOOR..HTTP_OK_CEILING
            val stream = if (succeeded) connection.inputStream else connection.errorStream
            return status to stream?.bufferedReader()?.use { it.readText() }.orEmpty()
        } finally {
            connection.disconnect()
        }
    }
}
