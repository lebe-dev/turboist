package ru.tinyops.turboist.nativeapp.unsent.ui

import androidx.annotation.StringRes
import ru.tinyops.turboist.core.sync.write.OutboxOpKind
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.unsent.UnsentReason

/**
 * What each queued write is called, and what each refusal reads as.
 *
 * Every write the API offers can end up in this list, so every one of them needs
 * a name a person can read — a row saying `task.bulk.priority` tells the user
 * nothing they can act on. The mapping is exhaustive by construction: the
 * compiler refuses this file if a write is added to the catalog and left
 * unnamed, which is what keeps a new op from turning up here as a blank.
 *
 * A `null` kind is a write queued by a build this one does not know. It is still
 * listed, because a change the app cannot describe is exactly the kind a user
 * must not lose without being told.
 */
@StringRes
fun labelFor(kind: OutboxOpKind?): Int =
    when (kind) {
        null -> R.string.native_unsent_op_unknown
        OutboxOpKind.TASK_CREATE -> R.string.native_unsent_op_taskCreate
        OutboxOpKind.TASK_PATCH -> R.string.native_unsent_op_taskPatch
        OutboxOpKind.TASK_DELETE -> R.string.native_unsent_op_taskDelete
        // The two the web client also queues, in the same words it uses.
        OutboxOpKind.TASK_COMPLETE -> R.string.offline_unsentOpComplete
        OutboxOpKind.TASK_UNCOMPLETE -> R.string.offline_unsentOpUncomplete
        OutboxOpKind.TASK_CANCEL -> R.string.native_unsent_op_taskCancel
        OutboxOpKind.TASK_MOVE -> R.string.native_unsent_op_taskMove
        OutboxOpKind.TASK_PLAN -> R.string.native_unsent_op_taskPlan
        OutboxOpKind.TASK_PIN -> R.string.native_unsent_op_taskPin
        OutboxOpKind.TASK_UNPIN -> R.string.native_unsent_op_taskUnpin
        OutboxOpKind.TASK_DUPLICATE -> R.string.native_unsent_op_taskDuplicate
        OutboxOpKind.TASK_DECOMPOSE -> R.string.native_unsent_op_taskDecompose
        OutboxOpKind.TASK_RELATION_ADD -> R.string.native_unsent_op_taskRelationAdd
        OutboxOpKind.TASK_RELATION_REMOVE -> R.string.native_unsent_op_taskRelationRemove
        OutboxOpKind.TASK_BULK_COMPLETE -> R.string.native_unsent_op_taskBulkComplete
        OutboxOpKind.TASK_BULK_MOVE -> R.string.native_unsent_op_taskBulkMove
        OutboxOpKind.TASK_BULK_PRIORITY -> R.string.native_unsent_op_taskBulkPriority
        OutboxOpKind.TASK_GROUP -> R.string.native_unsent_op_taskGroup
        OutboxOpKind.PROJECT_CREATE -> R.string.native_unsent_op_projectCreate
        OutboxOpKind.PROJECT_PATCH -> R.string.native_unsent_op_projectPatch
        OutboxOpKind.PROJECT_DELETE -> R.string.native_unsent_op_projectDelete
        OutboxOpKind.PROJECT_STATUS -> R.string.native_unsent_op_projectStatus
        OutboxOpKind.PROJECT_PIN -> R.string.native_unsent_op_projectPin
        OutboxOpKind.PROJECT_UNPIN -> R.string.native_unsent_op_projectUnpin
        OutboxOpKind.PROJECT_TROIKI -> R.string.native_unsent_op_projectTroiki
        OutboxOpKind.SECTION_CREATE -> R.string.native_unsent_op_sectionCreate
        OutboxOpKind.SECTION_PATCH -> R.string.native_unsent_op_sectionPatch
        OutboxOpKind.SECTION_DELETE -> R.string.native_unsent_op_sectionDelete
        OutboxOpKind.SECTION_REORDER -> R.string.native_unsent_op_sectionReorder
        OutboxOpKind.CONTEXT_CREATE -> R.string.native_unsent_op_contextCreate
        OutboxOpKind.CONTEXT_PATCH -> R.string.native_unsent_op_contextPatch
        OutboxOpKind.CONTEXT_DELETE -> R.string.native_unsent_op_contextDelete
        OutboxOpKind.LABEL_CREATE -> R.string.native_unsent_op_labelCreate
        OutboxOpKind.LABEL_PATCH -> R.string.native_unsent_op_labelPatch
        OutboxOpKind.LABEL_DELETE -> R.string.native_unsent_op_labelDelete
        OutboxOpKind.TEMPLATE_CREATE -> R.string.native_unsent_op_templateCreate
        OutboxOpKind.TEMPLATE_REPLACE -> R.string.native_unsent_op_templateReplace
        OutboxOpKind.TEMPLATE_DELETE -> R.string.native_unsent_op_templateDelete
        OutboxOpKind.TEMPLATE_INSTANTIATE -> R.string.native_unsent_op_templateInstantiate
        OutboxOpKind.SETTINGS_PATCH -> R.string.native_unsent_op_settingsPatch
        OutboxOpKind.APP_SETTINGS_AUTO_LABELS -> R.string.native_unsent_op_autoLabelRules
        OutboxOpKind.APP_SETTINGS_PROJECT_SUGGESTIONS -> R.string.native_unsent_op_projectSuggestionRules
        OutboxOpKind.STATE_PATCH -> R.string.native_unsent_op_statePatch
        OutboxOpKind.TROIKI_START -> R.string.native_unsent_op_troikiStart
        OutboxOpKind.TROIKI_RESET -> R.string.native_unsent_op_troikiReset
        OutboxOpKind.HARPOON_ATTACH -> R.string.native_unsent_op_harpoonAttach
        OutboxOpKind.HARPOON_DETACH -> R.string.native_unsent_op_harpoonDetach
    }

/** Why the server would not take the change, in one sentence. */
@StringRes
fun reasonFor(reason: UnsentReason): Int =
    when (reason) {
        UnsentReason.BLOCKED -> R.string.native_unsent_reason_blocked
        UnsentReason.TARGET_GONE -> R.string.native_unsent_reason_targetGone
        UnsentReason.LIMIT_REACHED -> R.string.native_unsent_reason_limitReached
        UnsentReason.PLACEMENT_REFUSED -> R.string.native_unsent_reason_placementRefused
        UnsentReason.TROIKI_SLOT_FULL -> R.string.native_unsent_reason_troikiSlotFull
        UnsentReason.INVALID -> R.string.native_unsent_reason_invalid
        UnsentReason.CONFLICT -> R.string.native_unsent_reason_conflict
        UnsentReason.NOT_ALLOWED -> R.string.native_unsent_reason_notAllowed
        UnsentReason.UNSENDABLE -> R.string.native_unsent_reason_unsendable
        UnsentReason.SERVER_ERROR -> R.string.native_unsent_reason_serverError
        UnsentReason.UNKNOWN -> R.string.native_unsent_reason_unknown
    }
