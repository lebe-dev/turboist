package ru.tinyops.turboist.nativeapp.labels

import kotlinx.coroutines.flow.MutableSharedFlow
import ru.tinyops.turboist.core.sync.write.LabelWriteRepo
import ru.tinyops.turboist.core.sync.write.WriteRefused
import javax.inject.Inject
import javax.inject.Singleton

/**
 * What is being changed about a label, with the fields left alone absent.
 *
 * Absent is not the same as empty: clearing a colour and not touching it are
 * different intentions, and a screen that could only send the whole record would
 * have to guess the parts it does not show.
 */
data class LabelEdit(
    val name: String? = null,
    val color: String? = null,
    val isFavourite: Boolean? = null,
    val isPrivate: Boolean? = null,
)

/**
 * The writes the label screens make.
 *
 * A port onto the shared write path rather than a second copy of it: each of
 * these applies its change to the replica and queues the request for the server
 * in one transaction, so a label created or renamed in a tunnel is right on
 * screen at once and right on the server whenever the phone comes back.
 *
 * Tagging a task is not here. That is a change to the task, made from the task
 * screens through their own port, so "put this label on this task" has one
 * meaning in the app rather than two.
 */
interface LabelActions {
    suspend fun createLabel(
        name: String,
        color: String,
        isFavourite: Boolean,
    )

    suspend fun editLabel(
        labelLocalId: Long,
        edit: LabelEdit,
    )

    /** Removes the label, and with it every tagging of it. The taggings are edges, not history. */
    suspend fun deleteLabel(labelLocalId: Long)
}

/**
 * Something a label screen has to say back after an action.
 *
 * Carried as a value rather than as words, so the screens stay free of resources
 * and of a language. There is one of them because there is one thing worth
 * saying: nothing here is refused by a rule the user could act on — a name
 * already taken is settled by the server, long after the gesture — so every
 * failure says the same thing, that the change did not happen.
 */
enum class LabelMessage {
    /** The change did not happen, and the user is not expected to know why. */
    FAILED,
}

/**
 * Runs a write and reports whatever it refuses with.
 *
 * A refusal is expected traffic rather than an error: it is reported to the
 * screen the user is looking at, not thrown past it.
 */
internal suspend fun MutableSharedFlow<LabelMessage>.reporting(write: suspend () -> Unit) {
    try {
        write()
    } catch (refusal: WriteRefused) {
        emit(LabelMessage.FAILED)
    } catch (failure: RuntimeException) {
        emit(LabelMessage.FAILED)
    }
}

/**
 * The label screens' writes, made against the shared write path.
 *
 * Nothing is decided here. The write path applies each change to the replica and
 * queues the request in one transaction; this only names the calls the screens
 * make.
 */
@Singleton
class WriteRepoLabelActions
    @Inject
    constructor(
        private val labels: LabelWriteRepo,
    ) : LabelActions {
        override suspend fun createLabel(
            name: String,
            color: String,
            isFavourite: Boolean,
        ) {
            labels.create(name = name, color = color, isFavourite = isFavourite)
        }

        override suspend fun editLabel(
            labelLocalId: Long,
            edit: LabelEdit,
        ) {
            labels.patch(
                labelLocalId = labelLocalId,
                name = edit.name,
                color = edit.color,
                isFavourite = edit.isFavourite,
                isPrivate = edit.isPrivate,
            )
        }

        override suspend fun deleteLabel(labelLocalId: Long) {
            labels.delete(labelLocalId)
        }
    }
