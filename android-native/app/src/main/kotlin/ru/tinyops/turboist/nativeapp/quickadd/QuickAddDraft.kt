package ru.tinyops.turboist.nativeapp.quickadd

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.TaskDestination
import java.time.LocalDate
import java.time.ZoneId

/**
 * A capture asked for before the surface that answers it exists.
 *
 * The launcher shortcut and a share from another app both arrive as this, and so
 * does a screen that opens the sheet already pointed at somewhere — a project
 * page, or one column of a board. Everything in it is a starting point the user
 * can still change; nothing here is committed to.
 */
data class QuickAddRequest(
    val title: String = "",
    val description: String = "",
    val projectLocalId: Long? = null,
    val sectionLocalId: Long? = null,
)

/**
 * What the user has typed so far.
 *
 * [titles] is the raw contents of the title field, which may hold several lines:
 * one line is one task, and capturing five things at once is a single gesture
 * rather than five trips through the sheet. Every other field is shared by all
 * of them — they were written down in one sitting, about one thing.
 *
 * A `null` [projectLocalId] means the inbox. It is the default because the inbox
 * is what capture is for: somewhere to put a thought without first deciding
 * where it belongs.
 */
data class QuickAddDraft(
    val titles: String = "",
    val description: String = "",
    val projectLocalId: Long? = null,
    val sectionLocalId: Long? = null,
    val priority: Priority = Priority.NONE,
    val dayPart: DayPart = DayPart.NONE,
    val dueDate: LocalDate? = null,
    val labelNames: List<String> = emptyList(),
    val rejectedAutoLabels: List<String> = emptyList(),
) {
    /** One task per non-blank line, in the order they were typed. */
    val titleLines: List<String>
        get() = titles.lines().map(String::trim).filter(String::isNotEmpty)

    /** True while the draft is headed for the inbox rather than for a project. */
    val isInbox: Boolean
        get() = projectLocalId == null && sectionLocalId == null

    /** True once there is something to create. */
    val canSubmit: Boolean
        get() = titleLines.isNotEmpty()

    /**
     * Where the tasks will be filed.
     *
     * A column wins over the project that owns it: picking a column is the more
     * specific answer, and the server files the task under both.
     */
    fun destination(): TaskDestination =
        when {
            sectionLocalId != null -> TaskDestination.InSection(sectionLocalId)
            projectLocalId != null -> TaskDestination.InProject(projectLocalId)
            else -> TaskDestination.Inbox
        }

    /**
     * The tasks this draft stands for, one per typed line.
     *
     * A due date is dropped for anything going to the inbox. The inbox holds raw
     * capture and a date is a plan: scheduling waits until the thought has a home,
     * which is the same rule the web client applies by hiding the date controls
     * there.
     *
     * The labels are sent by name — that is how the task endpoints refer to them
     * — and the auto-label rules are left to run on top, minus whatever the user
     * took off. Nothing invents a label the workspace does not have: an unknown
     * name is left in the request for the server to answer.
     */
    fun tasks(zone: ZoneId): List<NewTask> {
        val at = dueDate.takeUnless { isInbox }?.atStartOfDay(zone)?.toInstant()?.toEpochMilli()
        return titleLines.map { line ->
            NewTask(
                title = line,
                description = description.trim(),
                priority = priority,
                dueAt = at,
                dueHasTime = false,
                dayPart = dayPart,
                labels = labelNames,
                removedAutoLabels = rejectedAutoLabels,
            )
        }
    }

    /** Starts a draft from what another screen, or another app, asked for. */
    companion object {
        fun from(request: QuickAddRequest): QuickAddDraft =
            QuickAddDraft(
                titles = request.title,
                description = request.description,
                projectLocalId = request.projectLocalId,
                sectionLocalId = request.sectionLocalId,
            )
    }
}
