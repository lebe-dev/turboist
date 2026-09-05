package ru.tinyops.turboist.nativeapp.labels

import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.LabelTagging

/** A label with nothing set but the two things every label has. */
fun label(
    localId: Long,
    name: String = "label $localId",
    color: String = "",
    isFavourite: Boolean = false,
    isPrivate: Boolean = false,
): Label =
    Label(
        localId = localId,
        serverId = localId,
        name = name,
        color = color,
        isFavourite = isFavourite,
        isPrivate = isPrivate,
        createdAt = 0,
        updatedAt = 0,
    )

/**
 * One tagging, with only the facts a case is actually about set.
 *
 * Everything else is left at a default that says "ordinary open work", so an
 * assertion reads as a statement about the one column it changed.
 */
fun tagging(
    labelLocalId: Long,
    taggedAt: Long? = null,
    status: TaskStatus = TaskStatus.OPEN,
    dueAt: Long? = null,
    completedAt: Long? = null,
    projectLocalId: Long? = null,
): LabelTagging =
    LabelTagging(
        labelLocalId = labelLocalId,
        taggedAt = taggedAt,
        status = status,
        dueAt = dueAt,
        completedAt = completedAt,
        projectLocalId = projectLocalId,
    )

/**
 * Every write the label screens can make, remembered rather than performed.
 *
 * What each write does to the replica is settled where the write path lives, so
 * a check about a screen only has to say which write it asked for and with what.
 */
class RecordingLabelActions(
    private val refuse: Boolean = false,
) : LabelActions {
    val calls = mutableListOf<String>()

    override suspend fun createLabel(
        name: String,
        color: String,
        isFavourite: Boolean,
    ) {
        record("create($name,$color,$isFavourite)")
    }

    override suspend fun editLabel(
        labelLocalId: Long,
        edit: LabelEdit,
    ) {
        record("edit($labelLocalId,${edit.name},${edit.color},${edit.isFavourite},${edit.isPrivate})")
    }

    override suspend fun deleteLabel(labelLocalId: Long) {
        record("delete($labelLocalId)")
    }

    private fun record(call: String) {
        calls += call
        if (refuse) throw IllegalStateException("the write path refused this one")
    }
}
