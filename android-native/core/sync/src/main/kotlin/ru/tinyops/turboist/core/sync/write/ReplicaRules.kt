package ru.tinyops.turboist.core.sync.write

import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.model.UserSettings
import ru.tinyops.turboist.core.model.view.openBlockerLocalIds
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.AppSettingsDto
import ru.tinyops.turboist.core.network.dto.UserSettingsDto
import ru.tinyops.turboist.core.network.mapping.toAppSettings
import ru.tinyops.turboist.core.network.mapping.toUserSettings

/**
 * The handful of the server's rules the device repeats for itself.
 *
 * The server is the only place the rules truly live, and a write that gets one
 * wrong is corrected by the next page of changes. So this is not a second
 * implementation of the domain — it is the shortest list of rules whose
 * violation the user would otherwise only discover much later, when the queue
 * finally drained and the change they made came back undone.
 *
 * Three kinds of rule qualify:
 *
 * - **Refusals.** Completing a task something still blocks would be accepted on
 *   screen and rejected hours later. Repeating the check means the answer is
 *   "not yet, this is in the way" while the user is still looking at it.
 * - **Consequences.** Parking a task parks the work under it. If the device did
 *   not do that too, the subtasks would sit in today's list until the queue
 *   drained, contradicting the screen the user just left.
 * - **Capacities.** A shelf that holds ten, a slot that holds three. Letting an
 *   eleventh on and taking it away again later is worse than saying no now.
 *
 * Everything else the server decides alone.
 */
class ReplicaRules(private val db: TurboistDatabase) {
    /** The user's own preferences as last replicated, or the product's defaults before a first sync. */
    suspend fun userSettings(): UserSettings {
        val row = db.settings().userSettings() ?: return UserSettings()
        val dto =
            runCatching { TurboistJson.decodeFromString(UserSettingsDto.serializer(), row.payload) }
                .getOrElse { return UserSettings() }
        return dto.toUserSettings()
    }

    /** The installation's rules as last replicated. Absent before a first sync, which means no rules. */
    suspend fun appSettings(): AppSettings {
        val row = db.settings().appSettings() ?: return AppSettings()
        val dto =
            runCatching { TurboistJson.decodeFromString(AppSettingsDto.serializer(), row.payload) }
                .getOrElse { return AppSettings() }
        return dto.toAppSettings()
    }

    // --- blockers ------------------------------------------------------------

    /**
     * The still-open tasks standing in the way of this one.
     *
     * The rule itself is not written here. It is the one every list draws its
     * padlock from ([openBlockerLocalIds] over the edges and the parent links),
     * and a completion guard that answered it a second way would sooner or later
     * disagree with the padlock the user was looking at when they tapped. So this
     * gathers the facts the rule needs and hands them over.
     *
     * The facts are only the ones that can matter: the edges that still hold
     * something up, and the chain of parents above this task and above each of
     * those blockers. That is what the rule walks — a task inherits what blocks
     * the work above it, unless the blocker lives inside its own subtree, which
     * is the work itself.
     *
     * Completed *and cancelled* blockers are absent from the edges by
     * construction: a task abandoned rather than done still stops holding things
     * up, and treating it otherwise would deadlock everything behind it forever.
     */
    suspend fun openBlockerLocalIds(taskLocalId: Long): List<Long> {
        val task = db.tasks().byLocalId(taskLocalId) ?: return emptyList()
        val edges = db.taskRelations().openBlockEdges()
        if (edges.isEmpty()) return emptyList()
        val parents = parentLinksAbove(edges.map { it.blockerLocalId } + task.localId)
        return openBlockerLocalIds(taskLocalId, edges, parents)
    }

    /**
     * The parent link of every task above the ones named, and of the ones named.
     *
     * Read as chains rather than as the whole table: the rule only ever walks
     * upwards, and the tasks it walks up from are this task and the handful that
     * hold something up. Each chain is remembered as it is walked, so a shared
     * ancestor is read once.
     *
     * A parent chain cannot loop in the product, but these rows were copied from
     * a server, and a replica somehow left with a loop in it must still answer
     * rather than spin.
     */
    private suspend fun parentLinksAbove(taskLocalIds: Collection<Long>): Map<Long, Long> {
        val parents = HashMap<Long, Long>()
        val walked = mutableSetOf<Long>()
        for (start in taskLocalIds.toSet()) {
            var localId: Long? = start
            while (localId != null && walked.add(localId)) {
                val parent = db.tasks().byLocalId(localId)?.parentLocalId
                if (parent != null) parents[localId] = parent
                localId = parent
            }
        }
        return parents
    }

    /** Every task under this one, at any depth. */
    suspend fun descendants(taskLocalId: Long): List<TaskRow> {
        val out = mutableListOf<TaskRow>()
        val seen = mutableSetOf(taskLocalId)
        val queue = ArrayDeque<Long>()
        queue += taskLocalId
        while (queue.isNotEmpty()) {
            for (child in db.tasks().subtasksOf(queue.removeFirst())) {
                if (!seen.add(child.localId)) continue
                out += child
                queue += child.localId
            }
        }
        return out
    }

    // --- the two planning cascades ------------------------------------------

    /**
     * Parks every open subtask alongside the task being parked, at any depth, and
     * clears the day it was scheduled for.
     *
     * A subtask left with a due date would keep surfacing in the day views for
     * work that was explicitly put off, which is the opposite of what parking
     * means. Finished subtasks are history and are left alone; ones sitting in
     * the inbox are left alone too, because the backlog lives inside contexts and
     * moving them out is the parent's own placement decision, not this cascade's.
     */
    suspend fun cascadeBacklog(
        taskLocalId: Long,
        at: Long,
    ) {
        for (child in descendants(taskLocalId)) {
            if (!cascades(child)) continue
            db.tasks().update(
                child.copy(
                    planState = PlanState.BACKLOG,
                    dueAt = null,
                    dueHasTime = false,
                    updatedAt = at,
                ),
            )
        }
    }

    /**
     * Pulls every open subtask into the week alongside the task being planned.
     *
     * Committing to a piece of work is a statement about the whole of it, so its
     * subtasks must not stay behind in the backlog and show up there as separate,
     * context-less rows. Unlike parking, this keeps due dates: a subtask
     * scheduled for a concrete day inside the week is still valid planning.
     */
    suspend fun cascadeWeek(
        taskLocalId: Long,
        at: Long,
    ) {
        for (child in descendants(taskLocalId)) {
            if (!cascades(child) || child.planState == PlanState.WEEK) continue
            db.tasks().update(child.copy(planState = PlanState.WEEK, updatedAt = at))
        }
    }

    private fun cascades(child: TaskRow): Boolean = child.status == TaskStatus.OPEN && child.inboxId == null

    // --- the priority a place in the daily plan fixes -------------------------

    /**
     * The priority every open task of a project inherits from the bucket the
     * project stands in, or `null` when it stands in none.
     *
     * A project in a bucket decides the priority of the work inside it, and the
     * server refuses a direct priority edit on such a task outright. Answering
     * that here is what lets a screen lock the picker instead of accepting an
     * edit that is certain to come back undone.
     */
    suspend fun pinnedPriorityOf(projectLocalId: Long?): Priority? {
        val category = projectLocalId?.let { db.projects().byLocalId(it) }?.troikiCategory ?: return null
        return when (category) {
            TroikiCategory.IMPORTANT -> Priority.HIGH
            TroikiCategory.MEDIUM -> Priority.MEDIUM
            TroikiCategory.REST -> Priority.LOW
            TroikiCategory.UNKNOWN -> null
        }
    }

    /**
     * Gives every open task of a project the priority its bucket prescribes.
     *
     * The server does this the moment a project takes a place in the plan, or a
     * task is written into or moved into one. Without it here, a plan rearranged
     * with no connection would keep showing the priorities the tasks had before —
     * on the very screen the change was made on, and for as long as the queue
     * takes to drain.
     */
    suspend fun pinProjectPriority(
        projectLocalId: Long,
        priority: Priority,
        at: Long,
    ) {
        for (row in db.tasks().openInProject(projectLocalId)) {
            if (row.priority == priority) continue
            db.tasks().update(row.copy(priority = priority, updatedAt = at))
        }
    }

    // --- labels --------------------------------------------------------------

    /**
     * The labels a new task ends up with: what the user chose, plus what the
     * installation's rules attach for its title, minus what the user took off
     * again.
     *
     * Labels are resolved, never created: a name this device does not know stays
     * in the request for the server to answer and is simply not shown until it
     * comes back. Inventing a label locally would create a second one with the
     * same name the moment the write landed.
     */
    suspend fun labelsForTitle(
        title: String,
        explicitNames: List<String>?,
        removedAutoNames: List<String>,
        currentLocalIds: List<Long> = emptyList(),
    ): List<Long> {
        val chosen = linkedSetOf<Long>()
        if (explicitNames != null) {
            for (name in explicitNames) {
                db.labels().byName(name)?.let { chosen += it.localId }
            }
        } else {
            chosen += currentLocalIds
        }
        chosen += autoLabelLocalIds(title)
        for (name in removedAutoNames) {
            db.labels().byName(name)?.let { chosen -= it.localId }
        }
        return chosen.toList()
    }

    /**
     * The rule-attached labels an edit took off a task.
     *
     * An edit hands over the whole label set, so a label the installation's rules
     * attach to the title, which the task is carrying and the new set leaves out,
     * is one the user unticked. It has to be named as such or it comes straight
     * back: the rules run again on both sides the moment the write is applied,
     * and re-attach anything they match that was not refused by name.
     *
     * Only what the task actually carries is considered. A rule that starts
     * matching because the same edit changed the title attaches a label the user
     * has never seen, and leaving it out of the set is not a rejection of it.
     */
    suspend fun rejectedAutoLabels(
        title: String,
        taskLocalId: Long,
        chosenNames: List<String>,
    ): List<String> {
        val automatic = autoLabelLocalIds(title)
        if (automatic.isEmpty()) return emptyList()
        val carried = db.tasks().labelsOf(taskLocalId).map { it.labelLocalId }.toSet()
        val chosen = chosenNames.mapNotNull { db.labels().byName(it)?.localId }.toSet()
        return automatic
            .filter { it in carried && it !in chosen }
            .mapNotNull { db.labels().byLocalId(it)?.name }
    }

    /**
     * The labels the installation's rules attach to a title.
     *
     * A rule names labels by server id, because the rules document is the
     * server's and travels back to it unchanged. A rule naming a label this
     * device has never seen contributes nothing rather than failing the write —
     * the server applies the same rules authoritatively a moment later.
     */
    suspend fun autoLabelLocalIds(title: String): List<Long> {
        val rules = appSettings().autoLabels
        if (rules.isEmpty()) return emptyList()
        val matched = linkedSetOf<Long>()
        for (rule in rules) {
            if (rule.mask.isEmpty()) continue
            val hit =
                if (rule.ignoreCase) {
                    title.contains(rule.mask, ignoreCase = true)
                } else {
                    title.contains(rule.mask)
                }
            if (!hit) continue
            for (serverId in rule.labelIds) {
                db.labels().localIdForServerId(serverId)?.let { matched += it }
            }
        }
        return matched.toList()
    }

    // --- capacities ----------------------------------------------------------

    /** Refuses a task pin once the user's own cap is reached. */
    suspend fun assertRoomToPinTask() {
        val limit = userSettings().maxPinnedTasks
        if (db.tasks().countPinned() >= limit) throw WriteRefused.PinLimitReached(limit)
    }

    /** Refuses a project pin once the user's own cap is reached. */
    suspend fun assertRoomToPinProject() {
        val limit = userSettings().maxPinnedProjects
        if (db.projects().countPinned() >= limit) throw WriteRefused.PinLimitReached(limit)
    }

    /**
     * Refuses a project into a daily slot that is already full.
     *
     * The check uses the starting size of a slot, which is a constant of the
     * method. A slot can *earn* more room by work being finished in the tier
     * above it, and that counter is the server's — it is not part of the
     * replica, so the device does not guess at it. The effect is a check that
     * is never stricter than the server's and sometimes looser: the plainly
     * hopeless attempt is answered immediately, and anything subtler is left to
     * the server, which has the number.
     */
    suspend fun assertRoomInTroikiSlot(category: TroikiCategory) {
        val taken = db.projects().countInTroikiCategory(category)
        if (taken >= TROIKI_SLOT_BASE_CAPACITY) {
            throw WriteRefused.TroikiSlotFull(category, TROIKI_SLOT_BASE_CAPACITY)
        }
    }

    companion object {
        /**
         * How many projects a daily slot holds before the cycle has earned it
         * any more room. The same number for all three slots, which is what the
         * method prescribes.
         */
        const val TROIKI_SLOT_BASE_CAPACITY: Int = 3
    }
}
