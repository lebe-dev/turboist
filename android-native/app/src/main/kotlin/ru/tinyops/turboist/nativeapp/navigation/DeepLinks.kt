package ru.tinyops.turboist.nativeapp.navigation

import android.content.Intent

/**
 * The URI shapes the app answers to from outside.
 *
 * Only the app's own scheme is registered. An `https` filter would have to name
 * a host, and the host here is whichever server the user connected to — not
 * known until runtime, and not verifiable without an association file on it.
 * Claiming any host with a task path instead would put the app in the chooser
 * for unrelated sites.
 */
object DeepLinks {
    /** Scheme owned by this app. */
    const val SCHEME: String = "turboist"

    /** Everything below `turboist://task` addresses a single task. */
    const val TASK_BASE_PATH: String = "$SCHEME://task"

    /** The link that opens the task with the given **server** id. */
    fun task(serverId: Long): String = "$TASK_BASE_PATH/$serverId"
}

/**
 * The task a link intent names, or `null` when the intent is not one.
 *
 * Read here rather than left to the navigation graph because a link can arrive
 * when there is no graph to answer it — during sign-in, or before the app's own
 * tree has been composed — and something has to recognise it that early.
 */
fun Intent.taskLinkServerId(): Long? {
    if (action != Intent.ACTION_VIEW) return null
    val link = data?.toString() ?: return null
    if (!link.startsWith("${DeepLinks.TASK_BASE_PATH}/")) return null
    return link.removePrefix("${DeepLinks.TASK_BASE_PATH}/").toLongOrNull()
}
