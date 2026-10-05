package ru.tinyops.turboist.nativeapp.templates

import ru.tinyops.turboist.core.sync.write.TemplateDraft

/**
 * The writes the template surfaces can make.
 *
 * One port for both of them — the screen that keeps templates and the task
 * action that cuts a new one from work that exists — because they are the same
 * three writes: read a draft, save it, use it. Two ports would be one rule with
 * two implementations.
 */
interface TemplateActions {
    /** Saves a new template. */
    suspend fun create(draft: TemplateDraft)

    /**
     * Rewrites a template whole.
     *
     * Editing is a replace rather than a patch: the editor always submits the
     * complete structure, and a partial update of a nested list of lines has no
     * meaning anyone could agree on.
     */
    suspend fun replace(
        templateLocalId: Long,
        draft: TemplateDraft,
    )

    suspend fun delete(templateLocalId: Long)

    /**
     * Turns a template into real work in a project.
     *
     * @return the id this device holds the new task under, so the screen can
     *   offer to open it — the server has not been told yet, and may not be for
     *   hours.
     */
    suspend fun instantiate(
        templateLocalId: Long,
        projectLocalId: Long,
    ): Long

    /**
     * The template a task and its subtree would make, built from the replica.
     *
     * It is a draft, not a template: nothing exists until it is saved with
     * [create]. Reading it here rather than asking the server is what lets the
     * gesture work on a task written down a minute ago with no network.
     */
    suspend fun draftFromTask(taskLocalId: Long): TemplateDraft
}
