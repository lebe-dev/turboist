package ru.tinyops.turboist.nativeapp.quickadd

import android.content.Intent

/**
 * How long a captured title is allowed to be before the rest becomes description.
 *
 * A share can hand over an entire article. A task is a thing to do, and a list
 * whose rows are paragraphs is unreadable, so the overflow is kept — in the
 * description, where nothing is lost — rather than thrown away or left to stretch
 * every list in the app.
 */
private const val TITLE_MAX_CHARS = 120

/** Runs of whitespace, including the line breaks a title may not contain. */
private val WHITESPACE = Regex("\\s+")

/**
 * The text another app handed over, exactly as the platform delivers it.
 *
 * Two fields, because a share is two things often enough to matter: browsers and
 * readers send the page's name as the subject and its address as the text. Kept
 * as a plain value so the rule that turns them into a task can be read, and
 * checked, without an Intent anywhere near it.
 */
data class SharedText(
    val subject: String?,
    val text: String?,
)

/**
 * Turns shared text into the beginnings of a task.
 *
 * The rule in one sentence: the first thing that reads like a name becomes the
 * title, and everything else becomes the description.
 *
 * - A subject with text beside it is a name with a body — a shared page, almost
 *   always. The subject titles the task and the text goes underneath it.
 * - Text alone is split at its first non-blank line: that line is what somebody
 *   would have typed, the rest is context they did not.
 * - A title longer than [TITLE_MAX_CHARS] is cut at the last space that fits, so
 *   a shared paragraph does not become a row no list can draw. What was cut is
 *   kept, at the top of the description.
 *
 * The title is always a single line, whatever arrived: the sheet reads one line
 * as one task, and a share must not silently become several.
 */
fun quickAddRequest(shared: SharedText): QuickAddRequest {
    val subject = shared.subject.orEmpty().trim()
    val body = shared.text.orEmpty().trim()
    val head: String
    val rest: String
    when {
        subject.isNotEmpty() && body.isNotEmpty() -> {
            head = subject
            rest = body
        }

        subject.isNotEmpty() -> {
            head = subject
            rest = ""
        }

        else -> {
            val lines = body.lines()
            val first = lines.indexOfFirst { it.isNotBlank() }
            head = if (first < 0) "" else lines[first]
            rest = if (first < 0) "" else lines.drop(first + 1).joinToString("\n").trim()
        }
    }
    val title = head.replace(WHITESPACE, " ").trim()
    // Cut at the last space that still fits, so the title ends on a whole word.
    // A single word longer than the limit has no space to cut at and is cut
    // where the limit falls.
    val cut =
        if (title.length <= TITLE_MAX_CHARS) {
            title.length
        } else {
            title.take(TITLE_MAX_CHARS).lastIndexOf(' ').takeIf { it > 0 } ?: TITLE_MAX_CHARS
        }
    return QuickAddRequest(
        title = title.take(cut).trim(),
        description = listOf(title.drop(cut).trim(), rest).filter(String::isNotEmpty).joinToString("\n\n"),
    )
}

/**
 * The action the launcher shortcut sends.
 *
 * Its own action rather than the app's plain launch intent, so opening the app
 * and asking it to capture something are told apart without inspecting extras.
 */
const val ACTION_QUICK_ADD: String = "ru.tinyops.turboist.native.action.QUICK_ADD"

/**
 * What this intent is asking to capture, or `null` when it is asking for
 * something else entirely.
 *
 * Two things land here: the launcher shortcut, which asks for an empty sheet,
 * and a share from another app, which asks for one filled in. Anything else —
 * a plain launch, a task link — is not a capture and is left to the rest of the
 * app.
 */
fun Intent.quickAddRequest(): QuickAddRequest? {
    if (action == ACTION_QUICK_ADD) return QuickAddRequest()
    if (action != Intent.ACTION_SEND) return null
    if (type?.startsWith("text/") != true) return null
    val shared =
        SharedText(
            subject = getStringExtra(Intent.EXTRA_SUBJECT),
            text = getStringExtra(Intent.EXTRA_TEXT),
        )
    return quickAddRequest(shared)
}
