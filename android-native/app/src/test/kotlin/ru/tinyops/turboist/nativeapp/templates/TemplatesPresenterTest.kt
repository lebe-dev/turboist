package ru.tinyops.turboist.nativeapp.templates

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.TaskTemplate
import ru.tinyops.turboist.core.model.TaskTemplateSubtask
import ru.tinyops.turboist.core.sync.write.TemplateDraft
import ru.tinyops.turboist.core.sync.write.TemplateSubtaskDraft
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * What the templates screen does with the blueprints it is given.
 *
 * All of it runs without Compose, without a database and without a server: what
 * a template becomes when it is edited, and what is asked for when it is used,
 * are decisions about values.
 *
 * The case worth naming is the last one. The editor shows a template's name, its
 * description and its checklist, and saving is a replace — so a template whose
 * labels the editor cannot show must still come back with them, or using the app
 * from a phone would quietly strip work done on the web.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class TemplatesPresenterTest {
    private val templates = MutableStateFlow<List<TaskTemplate>>(emptyList())
    private val projects = MutableStateFlow(listOf(project(7, "Website")))
    private val labels = MutableStateFlow(listOf(label(3, "chore")))
    private val actions = RecordingTemplateActions()
    private val sync = CountingScheduler()

    private fun TestScope.presenter(): TemplatesPresenter {
        val presenter =
            TemplatesPresenter(backgroundScope, templates, projects, labels, actions, sync)
        backgroundScope.launch { presenter.state.collect {} }
        return presenter
    }

    @Test
    fun `a workspace not read yet is not the same as one with no templates`() =
        runTest {
            val presenter = presenter()

            assertTrue(presenter.state.value.loading)
            assertFalse(presenter.state.value.isEmpty, "nothing has been read, so nothing is known to be empty")
        }

    @Test
    fun `the editor opens on a template filled in with what it holds`() =
        runTest {
            val presenter = presenter()
            templates.value = listOf(template(1, "Release", lines = listOf("Tag the build", "Write the notes")))
            runCurrent()

            presenter.edit(presenter.state.value.templates.single())
            runCurrent()

            val editor = assertNotNull(presenter.state.value.editor)
            assertEquals(1L, editor.templateLocalId)
            assertEquals("Release", editor.name)
            assertContentEquals(listOf("Tag the build", "Write the notes"), editor.lines.map { it.title })
        }

    @Test
    fun `a new template is saved whole, with blank lines dropped as the server drops them`() =
        runTest {
            val presenter = presenter()

            presenter.startNewTemplate()
            presenter.rename("  Release  ")
            presenter.addLine()
            presenter.retitleLine(0, "Tag the build")
            presenter.addLine()
            presenter.save()
            runCurrent()

            val saved = assertNotNull(actions.created.singleOrNull())
            assertEquals("Release", saved.name, "a name is trimmed, as an empty one is refused")
            assertContentEquals(listOf("Tag the build"), saved.subtasks.map { it.title })
            assertNull(presenter.state.value.editor, "a saved template closes the editor")
        }

    @Test
    fun `an unnamed template is not saved and the editor stays open`() =
        runTest {
            val presenter = presenter()

            presenter.startNewTemplate()
            presenter.rename("   ")
            presenter.save()
            runCurrent()

            assertTrue(actions.created.isEmpty(), "the task every template makes has to be called something")
            assertNotNull(presenter.state.value.editor)
        }

    @Test
    fun `editing a template replaces the one it opened rather than adding another`() =
        runTest {
            val presenter = presenter()
            templates.value = listOf(template(1, "Release", lines = listOf("Tag the build")))
            runCurrent()

            presenter.edit(presenter.state.value.templates.single())
            presenter.rename("Release checklist")
            presenter.save()
            runCurrent()

            assertTrue(actions.created.isEmpty())
            val (templateLocalId, draft) = assertNotNull(actions.replaced.singleOrNull())
            assertEquals(1L, templateLocalId)
            assertEquals("Release checklist", draft.name)
        }

    @Test
    fun `what the editor does not show, it still carries`() =
        runTest {
            val presenter = presenter()
            templates.value =
                listOf(
                    template(
                        localId = 1,
                        name = "Release",
                        priority = Priority.HIGH,
                        dayPart = DayPart.MORNING,
                        labels = listOf(label(3, "chore")),
                    ),
                )
            runCurrent()

            presenter.edit(presenter.state.value.templates.single())
            presenter.describe("Everything a release needs")
            presenter.save()
            runCurrent()

            val draft = assertNotNull(actions.replaced.singleOrNull()).second
            assertEquals(Priority.HIGH, draft.priority)
            assertEquals(DayPart.MORNING, draft.dayPart)
            assertContentEquals(listOf(3L), draft.labelLocalIds)
        }

    @Test
    fun `a template that could not be saved says so and keeps what was typed`() =
        runTest {
            val presenter = presenter()
            actions.refuseWith = WriteRefused.Invalid("no")
            val said = mutableListOf<TemplateMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }

            presenter.startNewTemplate()
            presenter.rename("Release")
            presenter.save()
            runCurrent()

            assertContentEquals(listOf(TemplateMessage.SaveFailed), said)
            assertNotNull(presenter.state.value.editor, "nothing was saved, so nothing is lost either")
        }

    @Test
    fun `a draft cut from a task opens the editor on what the task holds`() =
        runTest {
            val presenter = presenter()
            actions.draft =
                TemplateDraft(
                    name = "Ship the site",
                    subtasks = listOf(TemplateSubtaskDraft(title = "Copy"), TemplateSubtaskDraft(title = "Photos")),
                )

            presenter.draftFromTask(42)
            runCurrent()

            val editor = assertNotNull(presenter.state.value.editor)
            assertNull(editor.templateLocalId, "nothing exists until the draft is saved")
            assertEquals("Ship the site", editor.name)
            assertContentEquals(listOf("Copy", "Photos"), editor.lines.map { it.title })
        }

    @Test
    fun `using a template makes the work in the chosen project and says how much`() =
        runTest {
            val presenter = presenter()
            templates.value = listOf(template(1, "Release", lines = listOf("Tag the build", "Write the notes")))
            runCurrent()
            val said = mutableListOf<TemplateMessage>()
            val opened = mutableListOf<Long>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            backgroundScope.launch { presenter.instantiated.collect { opened += it } }
            actions.instantiatedTaskLocalId = 99

            presenter.startInstantiate(presenter.state.value.templates.single())
            runCurrent()
            assertNotNull(presenter.state.value.choosingProjectFor, "the work has to go somewhere")

            presenter.instantiate(projectLocalId = 7)
            runCurrent()

            assertContentEquals(listOf(1L to 7L), actions.instantiated)
            assertContentEquals(listOf(TemplateMessage.Instantiated(3)), said, "the task and its two lines")
            assertContentEquals(listOf(99L), opened, "the screen goes to the work it just made")
            assertNull(presenter.state.value.choosingProjectFor)
        }

    @Test
    fun `a template that could not be used leaves the picker open`() =
        runTest {
            val presenter = presenter()
            templates.value = listOf(template(1, "Release"))
            runCurrent()
            val said = mutableListOf<TemplateMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            actions.refuseWith = WriteRefused.RowMissing("project", 7)

            presenter.startInstantiate(presenter.state.value.templates.single())
            presenter.instantiate(projectLocalId = 7)
            runCurrent()

            assertContentEquals(listOf(TemplateMessage.InstantiateFailed), said)
            assertNotNull(presenter.state.value.choosingProjectFor)
        }

    @Test
    fun `a template thrown away says so`() =
        runTest {
            val presenter = presenter()
            templates.value = listOf(template(1, "Release"))
            runCurrent()
            val said = mutableListOf<TemplateMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }

            presenter.delete(presenter.state.value.templates.single())
            runCurrent()

            assertContentEquals(listOf(1L), actions.deleted)
            assertContentEquals(listOf(TemplateMessage.Deleted), said)
        }

    // --- fakes and fixtures ---------------------------------------------------

    /** Every template write the screen can make, remembered rather than performed. */
    private class RecordingTemplateActions : TemplateActions {
        val created = mutableListOf<TemplateDraft>()
        val replaced = mutableListOf<Pair<Long, TemplateDraft>>()
        val deleted = mutableListOf<Long>()
        val instantiated = mutableListOf<Pair<Long, Long>>()
        var refuseWith: WriteRefused? = null
        var draft: TemplateDraft = TemplateDraft(name = "")
        var instantiatedTaskLocalId: Long = 1

        override suspend fun create(draft: TemplateDraft) {
            refuseWith?.let { throw it }
            created += draft
        }

        override suspend fun replace(
            templateLocalId: Long,
            draft: TemplateDraft,
        ) {
            refuseWith?.let { throw it }
            replaced += templateLocalId to draft
        }

        override suspend fun delete(templateLocalId: Long) {
            refuseWith?.let { throw it }
            deleted += templateLocalId
        }

        override suspend fun instantiate(
            templateLocalId: Long,
            projectLocalId: Long,
        ): Long {
            refuseWith?.let { throw it }
            instantiated += templateLocalId to projectLocalId
            return instantiatedTaskLocalId
        }

        override suspend fun draftFromTask(taskLocalId: Long): TemplateDraft {
            refuseWith?.let { throw it }
            return draft
        }
    }

    /** A sync engine nobody is supposed to need for any of this. */
    private class CountingScheduler : SyncScheduler {
        var requests = 0

        override suspend fun requestSyncNow() {
            requests++
        }
    }

    private fun template(
        localId: Long,
        name: String,
        lines: List<String> = emptyList(),
        priority: Priority = Priority.NONE,
        dayPart: DayPart = DayPart.NONE,
        labels: List<Label> = emptyList(),
    ): TaskTemplate =
        TaskTemplate(
            localId = localId,
            serverId = localId,
            name = name,
            priority = priority,
            dayPart = dayPart,
            labels = labels,
            subtasks =
                lines.mapIndexed { index, title ->
                    TaskTemplateSubtask(
                        localId = localId * 100 + index,
                        templateLocalId = localId,
                        position = index,
                        title = title,
                    )
                },
            createdAt = 0,
            updatedAt = 0,
        )

    private fun project(
        localId: Long,
        title: String,
    ): Project =
        Project(
            localId = localId,
            serverId = localId,
            contextLocalId = 1,
            title = title,
            createdAt = 0,
            updatedAt = 0,
        )

    private fun label(
        localId: Long,
        name: String,
    ): Label = Label(localId = localId, serverId = localId, name = name, createdAt = 0, updatedAt = 0)
}
