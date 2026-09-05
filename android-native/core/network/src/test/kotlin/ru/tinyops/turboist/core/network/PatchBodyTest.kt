package ru.tinyops.turboist.core.network

import ru.tinyops.turboist.core.network.dto.PatchTaskRequest
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * A PATCH body has to say three different things, and only two of them are
 * expressible with a nullable field. These tests pin the third.
 */
class PatchBodyTest {
    private fun encode(body: PatchTaskRequest): String =
        TurboistJson.encodeToString(PatchTaskRequest.serializer(), body)

    @Test
    fun `a field nobody touched is absent from the body`() {
        assertEquals("""{"title":"renamed"}""", encode(PatchTaskRequest(title = "renamed")))
    }

    @Test
    fun `clearing a field writes an explicit null, which is what makes it clear`() {
        assertEquals("""{"dueAt":null}""", encode(PatchTaskRequest(dueAt = Clearable.Clear)))
    }

    @Test
    fun `setting a clearable field writes the value like any other`() {
        assertEquals(
            """{"dueAt":"2024-03-09T12:34:56.789Z"}""",
            encode(PatchTaskRequest(dueAt = Clearable.Set("2024-03-09T12:34:56.789Z"))),
        )
    }

    @Test
    fun `the three states coexist in one body`() {
        val body =
            PatchTaskRequest(
                title = "renamed",
                dueAt = Clearable.Clear,
                recurrenceRule = Clearable.Set("FREQ=WEEKLY"),
            )

        assertEquals("""{"title":"renamed","dueAt":null,"recurrenceRule":"FREQ=WEEKLY"}""", encode(body))
    }

    @Test
    fun `a value or its absence maps onto set or clear in one step`() {
        assertEquals("""{"dueAt":null}""", encode(PatchTaskRequest(dueAt = Clearable.of(null))))
        assertEquals("""{"dueAt":"soon"}""", encode(PatchTaskRequest(dueAt = Clearable.of("soon"))))
    }

    @Test
    fun `a boolean that is genuinely false is still written`() {
        assertEquals("""{"isPrivate":false}""", encode(PatchTaskRequest(isPrivate = false)))
    }

    @Test
    fun `an empty patch is an empty body rather than a full overwrite`() {
        assertEquals("{}", encode(PatchTaskRequest()))
    }
}
