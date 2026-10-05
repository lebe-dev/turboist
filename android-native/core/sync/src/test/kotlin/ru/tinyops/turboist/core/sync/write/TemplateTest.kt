package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Priority
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * Templates: kept, cut from work that exists, and turned back into work.
 *
 * The cases are the ones the server settles when it does the same thing — which
 * fields a task made from a template carries, where it lands, what its subtasks
 * hang off, and what a draft cut from a task contains. They are checked here
 * because the device now answers them first: the tree appears the moment the
 * user asks for it, and an answer that disagreed with the server's would be
 * visibly rewritten minutes later.
 */
class TemplateTest : WriteTest() {
    @Test
    fun `a task made from a template carries the template's wording and fields`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val templateLocalId =
                givenTemplate(
                    TemplateDraft(
                        name = "Onboard client",
                        description = "Kick off a new engagement",
                        priority = Priority.HIGH,
                        dayPart = DayPart.MORNING,
                    ),
                )

            val write = templates.instantiate(templateLocalId, projectLocalId)

            val root = assertNotNull(db.tasks().byLocalId(write.entityLocalId))
            assertEquals("Onboard client", root.title, "the template's name is what the task is called")
            assertEquals("Kick off a new engagement", root.description)
            assertEquals(Priority.HIGH, root.priority)
            assertEquals(DayPart.MORNING, root.dayPart)
            assertNull(root.serverId, "the tree exists only here until the queue drains")
        }

    @Test
    fun `the tree lands in the project it was dropped into, under the context above it`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            val templateLocalId =
                givenTemplate(
                    TemplateDraft(
                        name = "Release",
                        subtasks =
                            listOf(
                                TemplateSubtaskDraft(title = "Tag the build"),
                                TemplateSubtaskDraft(title = "Write the notes"),
                            ),
                    ),
                )

            val write = templates.instantiate(templateLocalId, projectLocalId)

            val root = assertNotNull(db.tasks().byLocalId(write.entityLocalId))
            assertEquals(projectLocalId, root.projectLocalId)
            assertEquals(contextLocalId, root.contextLocalId, "a project always sits inside a context")
            assertNull(root.inboxId)
            val subtasks = db.tasks().subtasksOf(root.localId)
            assertContentEquals(listOf("Tag the build", "Write the notes"), subtasks.map { it.title })
            for (subtask in subtasks) {
                assertEquals(projectLocalId, subtask.projectLocalId, "work joins its parent where the parent is")
                assertEquals(contextLocalId, subtask.contextLocalId)
            }
        }

    @Test
    fun `each line of the checklist keeps its own fields`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val templateLocalId =
                givenTemplate(
                    TemplateDraft(
                        name = "Release",
                        subtasks =
                            listOf(
                                TemplateSubtaskDraft(
                                    title = "Tag the build",
                                    description = "From the release branch",
                                    priority = Priority.MEDIUM,
                                    dayPart = DayPart.EVENING,
                                ),
                            ),
                    ),
                )

            val write = templates.instantiate(templateLocalId, projectLocalId)

            val subtask = db.tasks().subtasksOf(write.entityLocalId).single()
            assertEquals("From the release branch", subtask.description)
            assertEquals(Priority.MEDIUM, subtask.priority)
            assertEquals(DayPart.EVENING, subtask.dayPart)
        }

    @Test
    fun `the labels a template names are attached, and so are the ones the rules attach`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val chore = givenLabel(name = "chore", serverId = 21)
            val bug = givenLabel(name = "bug", serverId = 22)
            givenAppSettings("""{"autoLabels":[{"mask":"fix","labelIds":[22],"ignoreCase":true}]}""")
            val templateLocalId =
                givenTemplate(
                    TemplateDraft(
                        name = "Fix the intake form",
                        labelLocalIds = listOf(chore),
                        subtasks = listOf(TemplateSubtaskDraft(title = "Reproduce it")),
                    ),
                )

            val write = templates.instantiate(templateLocalId, projectLocalId)

            val rootLabels = db.tasks().labelsOf(write.entityLocalId).map { it.labelLocalId }
            assertTrue(chore in rootLabels, "the template said so")
            assertTrue(bug in rootLabels, "the installation's rules say so about this title")
            val subtaskLabels = db.tasks().labelsOf(db.tasks().subtasksOf(write.entityLocalId).single().localId)
            assertTrue(subtaskLabels.isEmpty(), "a line that names no label and matches no rule gets none")
        }

    @Test
    fun `the whole tree is one queued write, naming the rows it drew`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val templateLocalId =
                givenTemplate(
                    TemplateDraft(
                        name = "Release",
                        subtasks =
                            listOf(
                                TemplateSubtaskDraft(title = "Tag the build"),
                                TemplateSubtaskDraft(title = "Write the notes"),
                            ),
                    ),
                )

            val write = templates.instantiate(templateLocalId, projectLocalId)

            val queued = queuedOps().filterIsInstance<InstantiateTemplateOp>()
            assertEquals(1, queued.size, "one gesture, one request")
            val op = queued.single()
            assertEquals(templateLocalId, op.templateLocalId)
            assertEquals(projectLocalId, op.projectLocalId)
            assertEquals(write.entityLocalId, op.rootTaskLocalId)
            assertContentEquals(
                db.tasks().subtasksOf(write.entityLocalId).map { it.localId },
                op.subtaskLocalIds,
                "the subtasks are named in the order the server will create them in",
            )
        }

    @Test
    fun `a template or a project that is not here refuses the whole thing`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val templateLocalId = givenTemplate(TemplateDraft(name = "Release"))

            assertFailsWith<WriteRefused.RowMissing> { templates.instantiate(404L, projectLocalId) }
            assertFailsWith<WriteRefused.RowMissing> { templates.instantiate(templateLocalId, 404L) }
            assertTrue(queue().none { it.op == OpNames.TEMPLATE_INSTANTIATE }, "nothing refused is queued")
        }

    @Test
    fun `a draft cut from a task flattens its whole subtree in reading order`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val root = givenTask(title = "Ship the site", serverId = 31, projectLocalId = projectLocalId)
            val first = givenTask(title = "Copy", serverId = 32, projectLocalId = projectLocalId, parentLocalId = root)
            givenTask(title = "Proofread", serverId = 33, projectLocalId = projectLocalId, parentLocalId = first)
            givenTask(title = "Photos", serverId = 34, projectLocalId = projectLocalId, parentLocalId = root)

            val draft = TemplateDrafts(db).fromTask(root)

            assertEquals("Ship the site", draft.name, "the task's title is what the template is called")
            assertContentEquals(
                listOf("Copy", "Proofread", "Photos"),
                draft.subtasks.map { it.title },
                "a template is one level deep, so the tree is read out in the order it is drawn",
            )
        }

    @Test
    fun `a draft carries the fields and labels of everything it captured`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val chore = givenLabel(name = "chore", serverId = 21)
            val root = givenTask(title = "Ship the site", serverId = 31, projectLocalId = projectLocalId)
            db.tasks().setLabels(root, listOf(chore), NOW)
            val child = givenTask(title = "Copy", serverId = 32, projectLocalId = projectLocalId, parentLocalId = root)
            db.tasks().update(
                assertNotNull(db.tasks().byLocalId(child)).copy(
                    description = "Two paragraphs",
                    priority = Priority.LOW,
                    dayPart = DayPart.AFTERNOON,
                ),
            )

            val draft = TemplateDrafts(db).fromTask(root)

            assertContentEquals(listOf(chore), draft.labelLocalIds)
            val line = draft.subtasks.single()
            assertEquals("Two paragraphs", line.description)
            assertEquals(Priority.LOW, line.priority)
            assertEquals(DayPart.AFTERNOON, line.dayPart)
        }

    @Test
    fun `drafting a task that is not here is refused, and writes nothing`() =
        runTest {
            assertFailsWith<WriteRefused.RowMissing> { TemplateDrafts(db).fromTask(404L) }
            assertTrue(queue().isEmpty())
        }

    /** A template already in the replica, saved the way the editor saves one. */
    private suspend fun givenTemplate(draft: TemplateDraft): Long = templates.create(draft).entityLocalId
}
