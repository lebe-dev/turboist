package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.RelativeDay
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * How the two halves of the completion history become one list.
 *
 * All of it is arithmetic on days and on identity, with no database, no server
 * and no screen: the point of the shaping layer is that the question "which day
 * is this task under, and is it already on screen" has one answer wherever it is
 * asked from.
 */
class CompletedHistoryTest {
    private val zone: ZoneId = ZoneId.of("UTC")
    private val today: LocalDate = LocalDate.of(2026, 3, 1)

    @Test
    fun `days run from the most recent backwards`() {
        val sections =
            completedDaySections(
                rows =
                    listOf(
                        replicaRow(1, at("2026-02-27T09:00:00Z")),
                        replicaRow(2, at("2026-03-01T08:00:00Z")),
                        replicaRow(3, at("2026-02-28T23:59:59Z")),
                    ),
                zone = zone,
                today = today,
            )

        assertEquals(
            listOf(LocalDate.of(2026, 3, 1), LocalDate.of(2026, 2, 28), LocalDate.of(2026, 2, 27)),
            sections.map { it.day },
        )
    }

    @Test
    fun `the three days with names of their own are marked as such`() {
        val sections =
            completedDaySections(
                rows =
                    listOf(
                        replicaRow(1, at("2026-03-01T08:00:00Z")),
                        replicaRow(2, at("2026-02-28T08:00:00Z")),
                        replicaRow(3, at("2026-01-04T08:00:00Z")),
                    ),
                zone = zone,
                today = today,
            )

        assertEquals(
            listOf(RelativeDay.TODAY, RelativeDay.YESTERDAY, RelativeDay.OTHER),
            sections.map { it.relative },
        )
    }

    @Test
    fun `a completion with no date is shown last rather than dropped`() {
        val sections =
            completedDaySections(
                rows = listOf(replicaRow(1, null), replicaRow(2, at("2026-03-01T08:00:00Z"))),
                zone = zone,
                today = today,
            )

        assertEquals(2, sections.size)
        assertNull(sections.last().day)
        assertEquals(listOf(1L), sections.last().rows.map { it.task.localId })
    }

    @Test
    fun `the order inside a day is the order the sources gave`() {
        val sections =
            completedDaySections(
                rows =
                    listOf(
                        replicaRow(1, at("2026-03-01T10:00:00Z")),
                        replicaRow(2, at("2026-03-01T09:00:00Z")),
                        replicaRow(3, at("2026-03-01T11:00:00Z")),
                    ),
                zone = zone,
                today = today,
            )

        assertEquals(listOf(1L, 2L, 3L), sections.single().rows.map { it.task.localId })
    }

    @Test
    fun `the device's own lines come first and are marked as its own`() {
        val rows =
            completedRows(
                replicated = listOf(finished(localId = 4, serverId = 40, at = at("2026-03-01T08:00:00Z"))),
                older = listOf(finished(localId = NO_LOCAL_ID, serverId = 90, at = at("2024-01-01T08:00:00Z"))),
                projectTitles = emptyMap(),
            )

        assertEquals(listOf(CompletedSource.REPLICA, CompletedSource.SERVER), rows.map { it.source })
        assertEquals(listOf("replica-4", "server-90"), rows.map { it.key })
    }

    @Test
    fun `a fetched line the device already holds is not drawn twice`() {
        val rows =
            completedRows(
                replicated = listOf(finished(localId = 4, serverId = 40, at = at("2026-03-01T08:00:00Z"))),
                older = listOf(finished(localId = NO_LOCAL_ID, serverId = 40, at = at("2026-03-01T08:00:00Z"))),
                projectTitles = emptyMap(),
            )

        assertEquals(1, rows.size)
        assertEquals(CompletedSource.REPLICA, rows.single().source)
    }

    @Test
    fun `a line names the project it lives in when the device knows that project`() {
        val rows =
            completedRows(
                replicated = listOf(finished(localId = 4, serverId = 40, at = 1, projectLocalId = 7)),
                older = emptyList(),
                projectTitles = mapOf(7L to "Website"),
            )

        assertEquals("Website", rows.single().projectTitle)
    }

    @Test
    fun `a fetched line whose project the device does not hold names none`() {
        val rows =
            completedRows(
                replicated = emptyList(),
                older = listOf(finished(localId = NO_LOCAL_ID, serverId = 90, at = 1, projectLocalId = NO_LOCAL_ID)),
                projectTitles = mapOf(7L to "Website"),
            )

        assertNull(rows.single().projectTitle)
        assertTrue(rows.single().key.startsWith("server-"))
    }

    private fun at(text: String): Long = Instant.parse(text).toEpochMilli()

    private fun replicaRow(
        localId: Long,
        at: Long?,
    ) = CompletedRow(finished(localId, localId, at), projectTitle = null, source = CompletedSource.REPLICA)

    private fun finished(
        localId: Long,
        serverId: Long?,
        at: Long?,
        projectLocalId: Long? = null,
    ) = task(
        localId = localId,
        serverId = serverId,
        status = TaskStatus.COMPLETED,
        completedAt = at,
        projectLocalId = projectLocalId,
    )
}
