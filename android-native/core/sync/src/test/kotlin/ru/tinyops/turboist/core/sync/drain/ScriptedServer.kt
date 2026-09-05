package ru.tinyops.turboist.core.sync.drain

import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.RecordedRequest
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.ApiHeaders

/**
 * A server that behaves the way the real one does about repeated writes.
 *
 * The whole point of sending a write under a key is what happens when the same
 * key arrives twice, and a server that simply answers whatever the test queued
 * next cannot show that. So this one keeps state: tasks it has been told about,
 * and the answer it gave to each key. A repeat of a key it has seen gets that
 * answer back, marked as a replay, and nothing happens a second time — which is
 * exactly the guarantee a drained queue depends on and exactly the thing that
 * would show up as a duplicate if the client got its keys wrong.
 *
 * Failures are scripted one at a time. A refusal is answered *before* the write
 * is applied and is not remembered, which is the real server's rule too: only a
 * successful answer is stored under its key, so a refusal is a refusal again on
 * every attempt. A break is the other shape — the write lands and the answer is
 * lost — and is the case a key exists for.
 */
class ScriptedServer(private val now: Long) : Dispatcher() {
    /**
     * One task as the server holds it.
     *
     * Where it sits is part of the record because some writes produce a shape
     * rather than a row — a template becomes a task with work under it — and a
     * shape can only be checked if the answer says what hangs off what.
     */
    data class Task(
        val id: Long,
        var title: String,
        var status: String = "open",
        var completedAt: String? = null,
        val inboxId: Long? = INBOX_ID,
        val projectId: Long? = null,
        val parentId: Long? = null,
    )

    /** One template as the server holds it: a task to make, and the ones to hang under it. */
    data class Template(
        val id: Long,
        val name: String,
        val subtaskTitles: List<String> = emptyList(),
    )

    /** What a test can make go wrong, one request at a time. */
    sealed interface Fault {
        /** The server understood and said no. Nothing is applied and nothing is remembered. */
        data class Refuse(val status: Int, val code: String, val details: String? = null) : Fault

        /** The write lands and the answer does not come back in a usable form. */
        data class LoseAnswer(val status: Int = HTTP_SERVER_ERROR) : Fault
    }

    /** Every request the server saw, in the order it saw them. */
    data class Seen(val method: String, val path: String, val key: String?, val body: String)

    val seen: MutableList<Seen> = mutableListOf()

    /** Consumed one per write; a test queues as many as the case needs. */
    val faults: ArrayDeque<Fault> = ArrayDeque()

    private val tasks = LinkedHashMap<Long, Task>()
    private val templates = LinkedHashMap<Long, Template>()

    /** The workspace this server holds, as ids: a context per id, and a project's context. */
    private val contexts = LinkedHashMap<Long, Long>()
    private val projects = LinkedHashMap<Long, Long>()
    private val blockedBy = mutableMapOf<Long, MutableList<Long>>()
    private val answers = mutableMapOf<String, Pair<Int, String>>()
    private var nextId = FIRST_SERVER_ID
    private var cursor = FIRST_CURSOR

    /** The tasks the server holds, oldest first — what "one copy each" is checked against. */
    fun tasks(): List<Task> = tasks.values.toList()

    fun task(id: Long): Task? = tasks[id]

    /**
     * Seeds a context and a project inside it, as they were before the test.
     *
     * They are seeded as a pair because the server holds them as one: a project
     * always sits inside a context, and a copy naming a project whose context was
     * never mentioned describes a workspace that cannot exist.
     */
    fun givenProject(
        id: Long,
        contextId: Long = id,
    ) {
        contexts[contextId] = contextId
        projects[id] = contextId
    }

    /** Seeds a template that was already there before the test started. */
    fun givenTemplate(
        id: Long,
        name: String,
        subtaskTitles: List<String> = emptyList(),
    ): Template = Template(id, name, subtaskTitles).also { templates[id] = it }

    /** Seeds a task that was already there before the test started. */
    fun givenTask(
        id: Long,
        title: String = "Already there",
    ): Task = Task(id, title).also { tasks[id] = it }

    /**
     * Records that [blocker] has to be finished before [blocked] can be.
     *
     * The server refuses to close a task anything open still stands in front of,
     * and a bulk request meets that rule one item at a time. A test about what a
     * device may decide on its own needs the real answer to compare against, so
     * the rule lives here rather than being assumed.
     */
    fun givenBlocks(
        blocker: Long,
        blocked: Long,
    ) {
        blockedBy.getOrPut(blocked) { mutableListOf() } += blocker
    }

    /** The writes the server was asked to make, ignoring the catch-up reads. */
    fun writes(): List<Seen> = seen.filterNot { it.path.startsWith(SYNC_PREFIX) }

    override fun dispatch(request: RecordedRequest): MockResponse {
        val path = request.url.encodedPath
        val key = request.headers[ApiHeaders.IDEMPOTENCY_KEY]
        seen += Seen(request.method, path, key, request.body?.utf8().orEmpty())

        if (path.startsWith(SYNC_PREFIX)) return read(path)

        val fault = faults.removeFirstOrNull()
        if (fault is Fault.Refuse) return error(fault.status, fault.code, fault.details)

        val remembered = key?.let { answers[it] }
        if (remembered != null) return json(remembered.second, remembered.first, replayed = true)

        val answer = apply(request, path)
        if (answer.first in SUCCESS) key?.let { answers[it] = answer }
        if (fault is Fault.LoseAnswer) return error(fault.status, "internal_error")
        return json(answer.second, answer.first)
    }

    // --- the writes -----------------------------------------------------------

    private fun apply(
        request: RecordedRequest,
        path: String,
    ): Pair<Int, String> {
        val body = request.body?.utf8().orEmpty()
        if (path == INBOX_TASKS) {
            val created = Task(nextId++, title = body.stringField("title") ?: "")
            tasks[created.id] = created
            cursor++
            return HTTP_CREATED to taskJson(created)
        }
        if (path == BULK_COMPLETE) return bulkComplete(body)
        if (path == BULK_MOVE || path == BULK_PRIORITY) return bulkTouch(body)
        if (path == GROUP) return group(body)
        if (path.startsWith(TEMPLATES)) return instantiate(request, path)
        val segments = path.removePrefix("$API_PREFIX/tasks/").split("/")
        val id = segments.firstOrNull()?.toLongOrNull() ?: return notFound()
        val task = tasks[id] ?: return notFound()
        cursor++
        // Removing a task answers with no body at all, which is what makes the
        // id in the request the only thing the client can get wrong here.
        if (request.method == "DELETE" && segments.size == 1) {
            tasks.remove(id)
            return HTTP_NO_CONTENT to ""
        }
        return when (segments.getOrNull(1)) {
            null -> {
                body.stringField("title")?.let { task.title = it }
                HTTP_OK to taskJson(task)
            }

            "complete" -> {
                task.status = "completed"
                task.completedAt = body.stringField("completedAt") ?: WireTime.format(now)
                HTTP_OK to taskJson(task)
            }

            "uncomplete" -> {
                task.status = "open"
                task.completedAt = null
                HTTP_OK to taskJson(task)
            }

            "move" -> {
                // The real server is given a whole placement, not its narrowest
                // part: a task in a project is also in that project's context,
                // and a body naming only the project is refused as incomplete.
                // The rule is repeated here so a client that forgets it fails a
                // test instead of only failing against a real instance.
                if (!body.namesField("inboxId") && !body.namesField("contextId")) {
                    HTTP_UNPROCESSABLE to errorBody("forbidden_placement")
                } else {
                    HTTP_OK to taskJson(task)
                }
            }

            "decompose" -> decompose(task, body)

            else -> HTTP_BAD_REQUEST to errorBody("unsupported_in_this_test")
        }
    }

    /**
     * Turns a template into a task with the template's checklist under it.
     *
     * The root is made first and the subtasks after it in the template's own
     * order, which is the real server's order too — and the only thing a client
     * can match the tree it drew itself against.
     */
    private fun instantiate(
        request: RecordedRequest,
        path: String,
    ): Pair<Int, String> {
        val id = path.removePrefix("$TEMPLATES/").removeSuffix("/instantiate").toLongOrNull() ?: return notFound()
        val template = templates[id] ?: return notFound()
        val projectId = request.body?.utf8().orEmpty().numberField("projectId") ?: return notFound()
        val root = Task(nextId++, title = template.name, inboxId = null, projectId = projectId)
        tasks[root.id] = root
        val subtasks =
            template.subtaskTitles.map { title ->
                val subtask = Task(nextId++, title = title, inboxId = null, projectId = projectId, parentId = root.id)
                tasks[subtask.id] = subtask
                subtask
            }
        cursor++
        val checklist = subtasks.joinToString(",") { taskJson(it) }
        return HTTP_CREATED to ("{\"root\":" + taskJson(root) + ",\"subtasks\":[" + checklist + "]}")
    }

    /**
     * Replaces a task with the ones cut from it, in the order the titles arrived.
     *
     * The task that was split is deleted, as the real server deletes it: the
     * pieces stand in its place rather than beside it.
     */
    private fun decompose(
        source: Task,
        body: String,
    ): Pair<Int, String> {
        val created =
            body.stringArrayField("titles").map { title ->
                val piece =
                    Task(
                        nextId++,
                        title = title,
                        inboxId = source.inboxId,
                        projectId = source.projectId,
                        parentId = source.parentId,
                    )
                tasks[piece.id] = piece
                piece
            }
        tasks.remove(source.id)
        return HTTP_CREATED to ("{\"created\":[" + created.joinToString(",") { taskJson(it) } + "]}")
    }

    /**
     * Finishes what it can of a selection and says so item by item.
     *
     * A task something open still stands in front of is refused on its own; the
     * rest of the selection goes through. That per-item answer is the shape the
     * whole bulk family uses, and it is why one refusal never costs the others.
     */
    private fun bulkComplete(body: String): Pair<Int, String> {
        val succeeded = mutableListOf<Long>()
        val failed = mutableListOf<Pair<Long, String>>()
        for (id in body.numberArrayField("ids")) {
            val task = tasks[id]
            when {
                task == null -> failed += id to "not_found"
                blockedBy[id].orEmpty().any { tasks[it]?.status == "open" } -> failed += id to "task_blocked"
                else -> {
                    task.status = "completed"
                    task.completedAt = WireTime.format(now)
                    succeeded += id
                }
            }
        }
        cursor++
        return HTTP_OK to bulkJson(succeeded, failed)
    }

    /** A bulk write whose effect this server does not model: it only says which ids it knows. */
    private fun bulkTouch(body: String): Pair<Int, String> {
        val ids = body.numberArrayField("ids")
        cursor++
        return HTTP_OK to bulkJson(ids.filter { it in tasks }, ids.filterNot { it in tasks }.map { it to "not_found" })
    }

    /** Creates the parent of a group and reports which children it took under it. */
    private fun group(body: String): Pair<Int, String> {
        val parent = Task(nextId++, title = body.stringField("title") ?: "")
        tasks[parent.id] = parent
        val children = body.numberArrayField("childIds")
        cursor++
        val failed = children.filterNot { it in tasks }.map { it to "not_found" }
        val succeeded = children.filter { it in tasks }
        return HTTP_CREATED to
            """{"parent":${taskJson(parent)},${bulkJson(succeeded, failed).removePrefix("{")}"""
    }

    // --- the reads ------------------------------------------------------------

    private fun read(path: String): MockResponse =
        if (path.endsWith("/snapshot")) json(snapshotJson(), HTTP_OK) else json(changesJson(), HTTP_OK)

    private fun snapshotJson(): String =
        """
        {"epoch":1,"cursor":$cursor,"completedSince":"${WireTime.format(now)}",
         "tasks":[${tasks.values.joinToString(",") { taskJson(it) }}],
         "projects":[${projects.entries.joinToString(",") { projectJson(it.key, it.value) }}],
         "sections":[],
         "contexts":[${contexts.keys.joinToString(",") { contextJson(it) }}],"labels":[],
         "taskRelations":[],
         "taskTemplates":[${templates.values.joinToString(",") { templateJson(it) }}],
         "userSettings":{"locale":"en"},
         "appSettings":{"autoLabels":[],"projectSuggestions":[]},
         "userState":{"activeContextId":null}}
        """.trimIndent()

    private fun changesJson(): String {
        val changes =
            tasks.values.mapIndexed { index, task ->
                """{"entity":"task","op":"upsert","seq":${index + 1},"id":${task.id},"data":${taskJson(task)}}"""
            }
        return """{"epoch":1,"cursor":$cursor,"hasMore":false,"changes":[${changes.joinToString(",")}]}"""
    }

    // --- shapes ---------------------------------------------------------------

    private fun taskJson(task: Task): String {
        val stamp = WireTime.format(now)
        return """{"id":${task.id},"title":"${task.title}","description":"","inboxId":${task.inboxId},""" +
            """"contextId":null,"projectId":${task.projectId},"sectionId":null,""" +
            """"parentId":${task.parentId},"sourceTaskId":null,""" +
            """"priority":"none","status":"${task.status}","dayPart":"none","planState":"none",""" +
            """"isPinned":false,"isPrivate":false,"isComplex":false,"postponeCount":0,"labels":[],""" +
            """"completedAt":${task.completedAt?.let { "\"$it\"" } ?: "null"},""" +
            """"blockedByCount":0,"relationCount":0,"createdAt":"$stamp","updatedAt":"$stamp"}"""
    }

    private fun contextJson(id: Long): String {
        val stamp = WireTime.format(now)
        return """{"id":$id,"name":"Work","color":"blue","isFavourite":false,""" +
            """"createdAt":"$stamp","updatedAt":"$stamp"}"""
    }

    private fun projectJson(
        id: Long,
        contextId: Long,
    ): String {
        val stamp = WireTime.format(now)
        return """{"id":$id,"contextId":$contextId,"title":"Website","description":"","color":"blue",""" +
            """"status":"active","projectType":"regular","isPinned":false,"isPrivate":false,""" +
            """"labels":[],"createdAt":"$stamp","updatedAt":"$stamp"}"""
    }

    private fun templateJson(template: Template): String {
        val stamp = WireTime.format(now)
        val lines =
            template.subtaskTitles.mapIndexed { index, title ->
                """{"id":${template.id * SUBTASK_ID_STRIDE + index},"title":"$title","description":"",""" +
                    """"priority":"none","dayPart":"none","labels":[]}"""
            }
        return """{"id":${template.id},"name":"${template.name}","description":"","priority":"none",""" +
            """"dayPart":"none","position":0,"labels":[],"subtasks":[${lines.joinToString(",")}],""" +
            """"createdAt":"$stamp","updatedAt":"$stamp"}"""
    }

    private fun notFound(): Pair<Int, String> = HTTP_NOT_FOUND to errorBody("not_found")

    private fun errorBody(
        code: String,
        details: String? = null,
    ): String {
        val detailsPart = if (details == null) "" else ""","details":$details"""
        return """{"error":{"code":"$code","message":"the server said no"$detailsPart}}"""
    }

    private fun error(
        status: Int,
        code: String,
        details: String? = null,
    ): MockResponse = json(errorBody(code, details), status)

    private fun json(
        body: String,
        code: Int,
        replayed: Boolean = false,
    ): MockResponse =
        MockResponse.Builder()
            .code(code)
            .setHeader("Content-Type", "application/json")
            .apply { if (replayed) setHeader(ApiHeaders.IDEMPOTENT_REPLAY, "true") }
            .body(body)
            .build()

    /** The one field this server reads out of a request body, without a parser. */
    private fun String.stringField(name: String): String? {
        val marker = "\"$name\":\""
        val start = indexOf(marker).takeIf { it >= 0 }?.plus(marker.length) ?: return null
        val end = indexOf('"', start).takeIf { it > start } ?: return null
        return substring(start, end)
    }

    /** A `"name":123` field, for the one number a body carries on its own. */
    private fun String.numberField(name: String): Long? {
        val marker = "\"$name\":"
        val start = indexOf(marker).takeIf { it >= 0 }?.plus(marker.length) ?: return null
        return substring(start).takeWhile { it.isDigit() }.toLongOrNull()
    }

    /** The strings of a `"name":["a","b"]` field, in the order the body wrote them. */
    private fun String.stringArrayField(name: String): List<String> {
        val marker = "\"$name\":["
        val start = indexOf(marker).takeIf { it >= 0 }?.plus(marker.length) ?: return emptyList()
        val end = indexOf(']', start).takeIf { it >= start } ?: return emptyList()
        return substring(start, end)
            .split(",")
            .map { it.trim().trim('"') }
            .filter { it.isNotEmpty() }
    }

    /** The per-item answer every bulk endpoint gives back. */
    private fun bulkJson(
        succeeded: List<Long>,
        failed: List<Pair<Long, String>>,
    ): String {
        val refused =
            failed.joinToString(",") { (id, code) ->
                """{"id":$id,"error":{"code":"$code","message":"the server said no"}}"""
            }
        return """{"succeeded":[${succeeded.joinToString(",")}],"failed":[$refused]}"""
    }

    /** The numbers of a `"name":[1,2,3]` field, in the order the body wrote them. */
    private fun String.numberArrayField(name: String): List<Long> {
        val marker = "\"$name\":["
        val start = indexOf(marker).takeIf { it >= 0 }?.plus(marker.length) ?: return emptyList()
        val end = indexOf(']', start).takeIf { it >= start } ?: return emptyList()
        return substring(start, end).split(",").mapNotNull { it.trim().toLongOrNull() }
    }

    /** Whether a request body carries a field at all, whatever its type. */
    private fun String.namesField(name: String): Boolean = contains("\"$name\":")

    companion object {
        const val HTTP_OK: Int = 200
        const val HTTP_CREATED: Int = 201
        const val HTTP_NO_CONTENT: Int = 204
        const val HTTP_BAD_REQUEST: Int = 400
        const val HTTP_NOT_FOUND: Int = 404
        const val HTTP_UNPROCESSABLE: Int = 422
        const val HTTP_CONFLICT: Int = 409
        const val HTTP_SERVER_ERROR: Int = 500

        const val API_PREFIX: String = "/api/v1"
        const val INBOX_TASKS: String = "$API_PREFIX/inbox/tasks"
        const val BULK_COMPLETE: String = "$API_PREFIX/tasks/bulk/complete"
        const val BULK_MOVE: String = "$API_PREFIX/tasks/bulk/move"
        const val BULK_PRIORITY: String = "$API_PREFIX/tasks/bulk/priority"
        const val GROUP: String = "$API_PREFIX/tasks/group"
        const val TEMPLATES: String = "$API_PREFIX/task-templates"

        /** Keeps a template's checklist ids apart from every other template's. */
        private const val SUBTASK_ID_STRIDE: Long = 1000
        const val SYNC_PREFIX: String = "$API_PREFIX/sync"

        /** The inbox is the same row on every instance. */
        const val INBOX_ID: Long = 2

        private const val FIRST_SERVER_ID: Long = 100
        private const val FIRST_CURSOR: Long = 10
        private val SUCCESS = 200..299
    }
}
