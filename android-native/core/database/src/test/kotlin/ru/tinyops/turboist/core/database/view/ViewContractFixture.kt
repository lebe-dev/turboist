package ru.tinyops.turboist.core.database.view

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.ProjectSectionRow
import ru.tinyops.turboist.core.database.entity.TaskRelationRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.INBOX_ID
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.model.WireTime
import java.io.File

/**
 * The dataset both implementations of the list views are read against.
 *
 * The file is shared with the server's own tests and lives outside this build,
 * which is the whole point: one dataset, one set of expected orderings, and no
 * way for the two implementations to agree with themselves while disagreeing
 * with each other.
 *
 * Records are addressed by a stable string key rather than by an id. Each side
 * assigns its own ids, so an expectation written in ids would only ever describe
 * the side that produced it.
 */
internal object ContractData {
    /** Where the shared files were placed, as the build tells the tests. */
    private val directory: File
        get() {
            val configured =
                System.getProperty("turboist.viewContract.dir")
                    ?: error("the build did not tell the tests where the shared view-contract files are")
            return File(configured)
        }

    fun fixture(): JsonObject = read("fixture.json")

    fun golden(view: String): JsonObject = read("golden/$view.json")

    private fun read(name: String): JsonObject {
        val file = File(directory, name)
        require(file.isFile) { "the shared view-contract file is missing: $file" }
        return Json.parseToJsonElement(file.readText()) as JsonObject
    }
}

/** The replica after the shared dataset has been written into it. */
internal class LoadedFixture(
    val clock: Map<String, Long>,
    val contexts: Map<String, Long>,
    val projects: Map<String, Long>,
    val sections: Map<String, Long>,
    val labels: Map<String, Long>,
    val tasks: Map<String, Long>,
    val declaredViews: List<String>,
) {
    private val keyOfTask: Map<Long, String> = tasks.entries.associate { (key, id) -> id to key }

    /** The instant the fixture named, or a failure if it named no such boundary. */
    fun instant(name: String): Long = clock[name] ?: error("the fixture declares no clock boundary named $name")

    /** One task, back in the fixture's own vocabulary. */
    fun keyOf(localId: Long): String = keyOfTask[localId] ?: error("a list returned a task the fixture never wrote")

    /** A rendered list, back in the fixture's own vocabulary. */
    fun keysOf(rows: List<TaskRow>): List<String> = rows.map { keyOf(it.localId) }
}

/**
 * Writes the shared dataset into [db].
 *
 * The rows go in exactly as the fixture describes them, rather than through the
 * write paths of the app. The fixture is a statement about stored rows, and the
 * question being asked is what the list queries make of those rows — putting a
 * write path in between would let a defect there hide a defect here, and would
 * make the two implementations disagree over data neither of them got wrong.
 */
internal suspend fun loadContractFixture(db: TurboistDatabase): LoadedFixture {
    val fixture = ContractData.fixture()
    val declaredClock = fixture.obj("clock")
    // The clock block also carries prose about what its boundaries mean, so only
    // the entries that name an instant are boundaries.
    val clock =
        declaredClock.keys
            .mapNotNull { name -> WireTime.parseOrNull(declaredClock.text(name))?.let { name to it } }
            .toMap()
    val now = clock["now"] ?: error("the fixture declares no current instant")

    val contexts = mutableMapOf<String, Long>()
    for (row in fixture.array("contexts")) {
        contexts[row.key()] =
            db.contexts().insert(
                ContextRow(
                    name = row.text("name") ?: "",
                    color = row.text("color") ?: "",
                    isFavourite = row.flag("isFavourite"),
                    createdAt = now,
                    updatedAt = now,
                ),
            )
    }

    val projects = mutableMapOf<String, Long>()
    for (row in fixture.array("projects")) {
        projects[row.key()] =
            db.projects().insert(
                ProjectRow(
                    contextLocalId = contexts.resolve(row.text("contextKey"), "context"),
                    title = row.text("title") ?: "",
                    color = row.text("color") ?: "",
                    status = ProjectStatus.fromWire(row.text("status") ?: ProjectStatus.OPEN.wire),
                    type = ProjectType.fromWire(row.text("type") ?: ProjectType.GENERIC.wire),
                    troikiCategory = row.text("troikiCategory")?.let(TroikiCategory::fromWire),
                    createdAt = now,
                    updatedAt = now,
                ),
            )
    }

    val sections = mutableMapOf<String, Long>()
    for (row in fixture.array("sections")) {
        sections[row.key()] =
            db.sections().insert(
                ProjectSectionRow(
                    projectLocalId = projects.resolve(row.text("projectKey"), "project"),
                    title = row.text("title") ?: "",
                    createdAt = now,
                    updatedAt = now,
                ),
            )
    }

    val labels = mutableMapOf<String, Long>()
    for (row in fixture.array("labels")) {
        labels[row.key()] =
            db.labels().insert(
                LabelRow(
                    name = row.text("name") ?: "",
                    color = row.text("color") ?: "",
                    createdAt = now,
                    updatedAt = now,
                ),
            )
    }

    val tasks = mutableMapOf<String, Long>()
    // A task can point at another one, as a subtask or as the snapshot a
    // recurrence left behind, and the replica checks that reference as the row
    // is written. So the pass repeats until nothing is left waiting, which also
    // means the fixture may list its tasks in whatever order reads best.
    var pending = fixture.array("tasks")
    while (pending.isNotEmpty()) {
        val stillWaiting = mutableListOf<JsonObject>()
        for (row in pending) {
            val placement = row.obj("placement")
            val parentKey = placement.text("parentKey")
            val sourceKey = row.text("recurrenceCompletionOf")
            if ((parentKey != null && parentKey !in tasks) || (sourceKey != null && sourceKey !in tasks)) {
                stillWaiting += row
                continue
            }
            val createdAt = WireTime.parse(row.text("createdAt") ?: error("every fixture task carries a creation time"))
            val localId =
                db.tasks().insert(
                    TaskRow(
                        title = row.text("title") ?: "",
                        description = row.text("description") ?: "",
                        inboxId = if (placement.flag("inbox")) INBOX_ID else null,
                        contextLocalId = placement.text("contextKey")?.let { contexts.resolve(it, "context") },
                        projectLocalId = placement.text("projectKey")?.let { projects.resolve(it, "project") },
                        sectionLocalId = placement.text("sectionKey")?.let { sections.resolve(it, "section") },
                        parentLocalId = parentKey?.let { tasks.resolve(it, "task") },
                        priority = Priority.fromWire(row.text("priority") ?: Priority.NONE.wire),
                        status = TaskStatus.fromWire(row.text("status") ?: TaskStatus.OPEN.wire),
                        dueAt = WireTime.parseOrNull(row.text("dueAt")),
                        dueHasTime = row.flag("dueHasTime"),
                        deadlineAt = WireTime.parseOrNull(row.text("deadlineAt")),
                        deadlineHasTime = row.flag("deadlineHasTime"),
                        dayPart = DayPart.fromWire(row.text("dayPart") ?: DayPart.NONE.wire),
                        planState = PlanState.fromWire(row.text("planState") ?: PlanState.NONE.wire),
                        isPinned = row.flag("isPinned"),
                        pinnedAt = WireTime.parseOrNull(row.text("pinnedAt")),
                        isPrivate = row.flag("isPrivate"),
                        isComplex = row.flag("isComplex"),
                        completedAt = WireTime.parseOrNull(row.text("completedAt")),
                        recurrenceRule = row.text("recurrenceRule"),
                        sourceTaskLocalId = sourceKey?.let { tasks.resolve(it, "task") },
                        troikiCategory = row.text("troikiCategory")?.let(TroikiCategory::fromWire),
                        createdAt = createdAt,
                        updatedAt = createdAt,
                    ),
                )
            tasks[row.key()] = localId
            val labelKeys = row.strings("labelKeys")
            if (labelKeys.isNotEmpty()) {
                db.tasks().setLabels(localId, labelKeys.map { labels.resolve(it, "label") }, createdAt)
            }
        }
        check(stillWaiting.size < pending.size) { "the fixture links its tasks in a circle and cannot be written" }
        pending = stillWaiting
    }

    for (row in fixture.array("relations")) {
        db.taskRelations().insert(
            TaskRelationRow(
                sourceTaskLocalId = tasks.resolve(row.text("sourceKey"), "task"),
                targetTaskLocalId = tasks.resolve(row.text("targetKey"), "task"),
                type = RelationType.fromWire(row.text("type") ?: RelationType.RELATED.wire),
                createdAt = now,
            ),
        )
    }

    return LoadedFixture(
        clock = clock,
        contexts = contexts,
        projects = projects,
        sections = sections,
        labels = labels,
        tasks = tasks,
        declaredViews = fixture.array("views").mapNotNull { it.text("name") },
    )
}

private fun JsonObject.obj(key: String): JsonObject = this[key] as? JsonObject ?: JsonObject(emptyMap())

private fun JsonObject.array(key: String): List<JsonObject> =
    (this[key] as? JsonArray)?.filterIsInstance<JsonObject>() ?: emptyList()

private fun JsonObject.strings(key: String): List<String> =
    (this[key] as? JsonArray)?.mapNotNull { (it as? JsonPrimitive)?.content } ?: emptyList()

private fun JsonObject.text(key: String): String? = (this[key] as? JsonPrimitive)?.takeIf { it.isString }?.content

private fun JsonObject.flag(key: String): Boolean = (this[key] as? JsonPrimitive)?.content?.toBoolean() ?: false

private fun JsonObject.key(): String = text("key") ?: error("every record of the fixture carries a key")

private fun Map<String, Long>.resolve(
    key: String?,
    kind: String,
): Long = this[key] ?: error("the fixture names a $kind that it never declares: $key")
