package ru.tinyops.turboist.nativeapp.templates

import ru.tinyops.turboist.core.sync.write.WriteRefused

/**
 * Making a template out of work that already exists, as one gesture.
 *
 * A narrow port of its own rather than the whole template catalogue, because the
 * screen that offers it — one task, with an action for turning it into a
 * blueprint — has no business listing, editing or using templates. Reading the
 * draft and saving it are one step here for the same reason: from the task's
 * side it is a single thing the user asked for.
 */
fun interface TemplateCapture {
    /** Saves the task and everything under it as a reusable template. */
    suspend fun captureFromTask(taskLocalId: Long)

    companion object {
        /**
         * What a screen composed without the template surface behind it gets.
         *
         * It refuses rather than quietly doing nothing: a gesture that reports
         * success and leaves no template behind is worse than one that says it
         * did not happen.
         */
        val Unavailable: TemplateCapture =
            TemplateCapture { throw WriteRefused.Invalid("this screen has no template surface behind it") }
    }
}
