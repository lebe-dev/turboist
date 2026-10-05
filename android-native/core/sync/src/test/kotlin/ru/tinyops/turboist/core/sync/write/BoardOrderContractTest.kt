package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Test
import java.io.File
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The board the user sees after a drag is the board the server will send back.
 *
 * Moving a column is applied to the replica immediately, with no connection, so
 * the ordering rule exists on both sides. If they disagree even slightly the
 * board settles once under the user's finger and then jumps again hours later,
 * when the queue finally drains — a correction nobody can connect to anything
 * they did.
 *
 * Both sides therefore replay one shared set of cases: the server runs them
 * through its own reorder, and this runs them through the write path, and both
 * must end with the same board. The file lives outside this build because it
 * belongs to neither implementation.
 */
class BoardOrderContractTest : WriteTest() {
    @Test
    fun `every shared case leaves the board the server would leave`() =
        runTest {
            val cases = sharedCases()
            assertTrue(cases.isNotEmpty(), "the shared board-ordering cases are empty")

            for (case in cases) {
                val contextLocalId = givenContext(name = case.name, serverId = null)
                val projectLocalId = givenProject(contextLocalId, title = case.name, serverId = null)
                // Columns are created in the order the case draws them, so each
                // one lands after the last and the board starts as 0..n-1.
                val ids =
                    case.board.associateWith { title ->
                        projects.createSection(projectLocalId, title).entityLocalId
                    }

                projects.reorderSection(ids.getValue(case.move), case.position)

                val board = db.sections().forProject(projectLocalId)
                assertEquals(case.expected, board.map { it.title }, "board after ${case.name}")
                assertEquals(
                    case.expected.indices.toList(),
                    board.map { it.position },
                    "positions after ${case.name}",
                )
            }
        }

    /** One case as the shared file states it. */
    private data class BoardCase(
        val name: String,
        val board: List<String>,
        val move: String,
        val position: Int,
        val expected: List<String>,
    )

    private fun sharedCases(): List<BoardCase> {
        val directory =
            System.getProperty("turboist.syncContract.dir")
                ?: error("the build did not tell the tests where the shared contract files are")
        val file = File(directory, "section-reorder.json")
        require(file.isFile) { "the shared board-ordering cases are missing: $file" }
        val parsed = Json.parseToJsonElement(file.readText()) as JsonObject
        return parsed.getValue("cases").jsonArray.map { element ->
            val case = element.jsonObject
            BoardCase(
                name = case.getValue("name").jsonPrimitive.content,
                board = case.getValue("board").jsonArray.map { it.jsonPrimitive.content },
                move = case.getValue("move").jsonPrimitive.content,
                position = case.getValue("position").jsonPrimitive.content.toInt(),
                expected = case.getValue("expected").jsonArray.map { it.jsonPrimitive.content },
            )
        }
    }
}
