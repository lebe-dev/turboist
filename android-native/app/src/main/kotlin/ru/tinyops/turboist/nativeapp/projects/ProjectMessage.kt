package ru.tinyops.turboist.nativeapp.projects

import kotlinx.coroutines.flow.MutableSharedFlow
import ru.tinyops.turboist.core.sync.write.WriteRefused

/**
 * Something a project screen has to say back after an action.
 *
 * Carried as a value rather than as words, so the screens stay free of resources
 * and of a language; the screen turns it into a sentence. Only refusals the user
 * can act on are told apart — a full shelf, a full slot, work that is still in
 * the way — because those name the thing to change. Everything else says the
 * same thing, that the change did not happen, and inventing a different sentence
 * for each would be noise rather than help.
 */
sealed interface ProjectMessage {
    /** The task cannot be completed while something still stands in its way. */
    data object Blocked : ProjectMessage

    /** The pinned shelf is full; [limit] is the user's own setting for it. */
    data class PinLimitReached(val limit: Int) : ProjectMessage

    /** The slot of the daily plan already holds as many projects as it takes. */
    data class TroikiSlotFull(val capacity: Int) : ProjectMessage

    /** The change did not happen, and the user is not expected to know why. */
    data object Failed : ProjectMessage
}

/**
 * Runs a write and turns whatever it refuses with into something the user can
 * read.
 *
 * A refusal is expected traffic rather than an error: the device repeats a few
 * of the server's rules precisely so the user meets them while they are still
 * looking at the thing they tried to change. So it is reported, not thrown.
 */
internal suspend fun MutableSharedFlow<ProjectMessage>.reporting(write: suspend () -> Unit) {
    try {
        write()
    } catch (refusal: WriteRefused) {
        emit(
            when (refusal) {
                is WriteRefused.TaskBlocked -> ProjectMessage.Blocked
                is WriteRefused.PinLimitReached -> ProjectMessage.PinLimitReached(refusal.limit)
                is WriteRefused.TroikiSlotFull -> ProjectMessage.TroikiSlotFull(refusal.capacity)
                else -> ProjectMessage.Failed
            },
        )
    } catch (failure: RuntimeException) {
        emit(ProjectMessage.Failed)
    }
}
