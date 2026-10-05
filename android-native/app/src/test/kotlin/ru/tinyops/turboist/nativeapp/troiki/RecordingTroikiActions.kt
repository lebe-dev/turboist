package ru.tinyops.turboist.nativeapp.troiki

import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.sync.write.WriteRefused

/**
 * Every write the daily plan can make, remembered rather than performed.
 *
 * What each write does to the copied data is settled where the write path
 * lives, so a check about the plan only has to say which write it asked for and
 * with what — and, when the write path would refuse, that the refusal is passed
 * on rather than swallowed.
 */
class RecordingTroikiActions : TroikiActions {
    val calls = mutableListOf<String>()
    var refuseWith: WriteRefused? = null

    private fun record(call: String) {
        calls += call
        refuseWith?.let { throw it }
    }

    override suspend fun setCategory(
        projectLocalId: Long,
        category: TroikiCategory?,
    ) = record("setCategory $projectLocalId ${category?.wire}")

    override suspend fun start() = record("start")

    override suspend fun reset() = record("reset")

    override suspend fun addTask(
        projectLocalId: Long,
        title: String,
    ) = record("addTask $projectLocalId $title")
}
