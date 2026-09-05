package ru.tinyops.turboist.core.model

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The expected strings below are the server's own enum values. They are spelled
 * out literally rather than derived, so that a rename on either side shows up
 * here as a failing test instead of as a task that silently loses its priority.
 */
class WireEnumTest {
    @Test
    fun `priority carries the server's values`() {
        assertWireValues(Priority.known, "high", "medium", "low", "no-priority")
    }

    @Test
    fun `task status carries the server's values`() {
        assertWireValues(TaskStatus.known, "open", "completed", "cancelled")
    }

    @Test
    fun `project status carries the server's values`() {
        assertWireValues(ProjectStatus.known, "open", "completed", "archived", "cancelled")
    }

    @Test
    fun `project type carries the server's values`() {
        assertWireValues(ProjectType.known, "generic", "software")
    }

    @Test
    fun `plan state carries the server's values`() {
        assertWireValues(PlanState.known, "none", "week", "backlog")
    }

    @Test
    fun `day part carries the server's values`() {
        assertWireValues(DayPart.known, "none", "morning", "afternoon", "evening")
    }

    @Test
    fun `troiki category carries the server's values`() {
        assertWireValues(TroikiCategory.known, "important", "medium", "rest")
    }

    @Test
    fun `relation type carries the server's values`() {
        assertWireValues(RelationType.known, "related", "blocks")
    }

    @Test
    fun `relation direction carries the server's values`() {
        assertWireValues(RelationDirection.known, "outgoing", "incoming")
    }

    @Test
    fun `client kind carries the server's values`() {
        assertWireValues(ClientKind.known, "web", "ios", "cli", "android")
    }

    @Test
    fun `harpoon kind carries the server's values`() {
        assertWireValues(HarpoonKind.known, "task", "project")
    }

    @Test
    fun `every constant round-trips through its wire value`() {
        for (constant in allKnownConstants()) {
            val decoded = decodeSameType(constant)
            assertEquals(constant, decoded, "round-trip of ${constant.javaClass.simpleName}.$constant")
        }
    }

    @Test
    fun `a value this client does not know decodes to the sentinel`() {
        assertEquals(Priority.UNKNOWN, Priority.fromWire("critical"))
        assertEquals(TaskStatus.UNKNOWN, TaskStatus.fromWire("archived"))
        assertEquals(ProjectStatus.UNKNOWN, ProjectStatus.fromWire("paused"))
        assertEquals(ProjectType.UNKNOWN, ProjectType.fromWire("hardware"))
        assertEquals(PlanState.UNKNOWN, PlanState.fromWire("someday"))
        assertEquals(DayPart.UNKNOWN, DayPart.fromWire("night"))
        assertEquals(TroikiCategory.UNKNOWN, TroikiCategory.fromWire("huge"))
        assertEquals(RelationType.UNKNOWN, RelationType.fromWire("duplicates"))
        assertEquals(RelationDirection.UNKNOWN, RelationDirection.fromWire("sideways"))
        assertEquals(ClientKind.UNKNOWN, ClientKind.fromWire("desktop"))
        assertEquals(HarpoonKind.UNKNOWN, HarpoonKind.fromWire("label"))
    }

    @Test
    fun `absent and blank values decode to the sentinel`() {
        assertEquals(Priority.UNKNOWN, Priority.fromWire(null))
        assertEquals(DayPart.UNKNOWN, DayPart.fromWire(""))
        assertEquals(TaskStatus.UNKNOWN, TaskStatus.fromWire(null))
    }

    @Test
    fun `the sentinel is the only constant with an empty wire value`() {
        for (constant in allConstants()) {
            val empty = (constant as WireEnum).wire.isEmpty()
            assertEquals(constant.name == "UNKNOWN", empty, "${constant.javaClass.simpleName}.${constant.name}")
        }
    }

    @Test
    fun `the strict decoder answers null instead of the sentinel`() {
        assertNull(Priority.fromWireOrNull("critical"))
        assertNull(Priority.fromWireOrNull(null))
        assertNull(DayPart.fromWireOrNull(""))
        assertEquals(Priority.HIGH, Priority.fromWireOrNull("high"))
    }

    @Test
    fun `the sentinel is never offered as a choice`() {
        for (list in knownLists()) {
            assertTrue(list.none { it.wire.isEmpty() }, "known must not offer the sentinel")
        }
    }

    private fun assertWireValues(
        known: List<WireEnum>,
        vararg expected: String,
    ) {
        assertEquals(expected.toList(), known.map { it.wire })
    }

    private fun knownLists(): List<List<WireEnum>> =
        listOf(
            Priority.known,
            TaskStatus.known,
            ProjectStatus.known,
            ProjectType.known,
            PlanState.known,
            DayPart.known,
            TroikiCategory.known,
            RelationType.known,
            RelationDirection.known,
            ClientKind.known,
            HarpoonKind.known,
        )

    private fun allKnownConstants(): List<WireEnum> = knownLists().flatten()

    private fun allConstants(): List<Enum<*>> =
        listOf(
            Priority.entries,
            TaskStatus.entries,
            ProjectStatus.entries,
            ProjectType.entries,
            PlanState.entries,
            DayPart.entries,
            TroikiCategory.entries,
            RelationType.entries,
            RelationDirection.entries,
            ClientKind.entries,
            HarpoonKind.entries,
        ).flatten()

    private fun decodeSameType(constant: WireEnum): WireEnum =
        when (constant) {
            is Priority -> Priority.fromWire(constant.wire)
            is TaskStatus -> TaskStatus.fromWire(constant.wire)
            is ProjectStatus -> ProjectStatus.fromWire(constant.wire)
            is ProjectType -> ProjectType.fromWire(constant.wire)
            is PlanState -> PlanState.fromWire(constant.wire)
            is DayPart -> DayPart.fromWire(constant.wire)
            is TroikiCategory -> TroikiCategory.fromWire(constant.wire)
            is RelationType -> RelationType.fromWire(constant.wire)
            is RelationDirection -> RelationDirection.fromWire(constant.wire)
            is ClientKind -> ClientKind.fromWire(constant.wire)
            is HarpoonKind -> HarpoonKind.fromWire(constant.wire)
            else -> error("unhandled enum ${constant.javaClass.name}")
        }
}
