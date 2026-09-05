package ru.tinyops.turboist.nativeapp.templates

import ru.tinyops.turboist.core.sync.write.TemplateDraft
import ru.tinyops.turboist.core.sync.write.TemplateDrafts
import ru.tinyops.turboist.core.sync.write.TemplateWriteRepo
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The template surfaces' writes, made against the shared write path.
 *
 * Nothing is decided here. Every call applies its change to the replica and
 * queues it for the server in one transaction; this class only names the writes
 * the template screens make.
 */
@Singleton
class WriteRepoTemplateActions
    @Inject
    constructor(
        private val templates: TemplateWriteRepo,
        private val drafts: TemplateDrafts,
    ) : TemplateActions, TemplateCapture {
        override suspend fun create(draft: TemplateDraft) {
            templates.create(draft)
        }

        override suspend fun replace(
            templateLocalId: Long,
            draft: TemplateDraft,
        ) {
            templates.replace(templateLocalId, draft)
        }

        override suspend fun delete(templateLocalId: Long) {
            templates.delete(templateLocalId)
        }

        override suspend fun instantiate(
            templateLocalId: Long,
            projectLocalId: Long,
        ): Long = templates.instantiate(templateLocalId, projectLocalId).entityLocalId

        override suspend fun draftFromTask(taskLocalId: Long): TemplateDraft = drafts.fromTask(taskLocalId)

        /**
         * Cuts a template out of a task and saves it in one step, which is what
         * the task screen's single gesture means. Both halves read and write the
         * replica only, so it works with no network like every other write here.
         */
        override suspend fun captureFromTask(taskLocalId: Long) {
            templates.create(drafts.fromTask(taskLocalId))
        }
    }
