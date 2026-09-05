package turboist.shrinker

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class ReflectionSurfacesTest {
    private fun of(source: String) = ReflectionSurfaces.of(source.trimIndent())

    @Test
    fun `a serializable class is a surface`() {
        val surfaces = of(
            """
            package ru.tinyops.turboist.core.network.dto

            import kotlinx.serialization.Serializable

            @Serializable
            data class TaskDto(val id: Long)
            """,
        )

        assertEquals(
            listOf(ReflectionSurfaces.Surface("ru.tinyops.turboist.core.network.dto.TaskDto", ReflectionSurfaces.Reason.SERIALIZATION)),
            surfaces,
        )
    }

    @Test
    fun `a class nothing looks up by name is not a surface`() {
        assertEquals(
            emptyList(),
            of(
                """
                package ru.tinyops.turboist.core.sync

                class SyncCycle(private val clock: Clock) {
                    fun run() = Unit
                }
                """,
            ),
        )
    }

    @Test
    fun `a nested serializable class is named the way the shrinker names it`() {
        val surfaces = of(
            """
            package ru.tinyops.turboist.core.sync.write

            import kotlinx.serialization.Serializable

            @Serializable
            sealed interface OutboxOp {
                @Serializable
                data class Complete(val taskId: Long) : OutboxOp
            }
            """,
        )

        assertEquals(
            listOf(
                "ru.tinyops.turboist.core.sync.write.OutboxOp",
                "ru.tinyops.turboist.core.sync.write.OutboxOp\$Complete",
            ),
            surfaces.map { it.className },
        )
    }

    @Test
    fun `the generated database implementation is what has to survive, not the class it is written beside`() {
        val surfaces = of(
            """
            package ru.tinyops.turboist.core.database

            import androidx.room.Database

            @Database(entities = [], version = 1)
            abstract class TurboistDatabase : RoomDatabase()
            """,
        )

        assertEquals(
            listOf(ReflectionSurfaces.Surface("ru.tinyops.turboist.core.database.TurboistDatabase_Impl", ReflectionSurfaces.Reason.ROOM)),
            surfaces,
        )
    }

    @Test
    fun `an annotation whose arguments run over many lines still attaches to its class`() {
        // A database lists every table it holds, so the annotation reaches the
        // class it is written on several lines later.
        val surfaces = of(
            """
            package ru.tinyops.turboist.core.database

            import androidx.room.Database

            @Database(
                entities = [
                    TaskRow::class,
                    ProjectRow::class,
                ],
                version = 1,
            )
            @TypeConverters(ReplicaConverters::class)
            abstract class TurboistDatabase : RoomDatabase()
            """,
        )

        assertEquals(listOf("ru.tinyops.turboist.core.database.TurboistDatabase_Impl"), surfaces.map { it.className })
    }

    @Test
    fun `an interface in a file that describes HTTP calls is a surface`() {
        val surfaces = of(
            """
            package ru.tinyops.turboist.core.network.api

            import retrofit2.http.GET

            interface TaskApi {
                @GET("/api/v1/tasks")
                suspend fun tasks(): List<TaskDto>
            }
            """,
        )

        assertEquals(
            listOf(ReflectionSurfaces.Surface("ru.tinyops.turboist.core.network.api.TaskApi", ReflectionSurfaces.Reason.RETROFIT)),
            surfaces,
        )
    }

    @Test
    fun `a class in a file that describes HTTP calls is not itself one`() {
        val surfaces = of(
            """
            package ru.tinyops.turboist.core.network.api

            import retrofit2.http.GET

            interface TaskApi {
                @GET("/api/v1/tasks")
                suspend fun tasks(): List<TaskDto>
            }

            class TaskApiDefaults
            """,
        )

        assertEquals(listOf("ru.tinyops.turboist.core.network.api.TaskApi"), surfaces.map { it.className })
    }

    @Test
    fun `an annotation only attaches to the declaration that follows it`() {
        // A blank line, a comment or any other statement ends the run, so an
        // annotation on one class must never be credited to the next one.
        val surfaces = of(
            """
            package ru.tinyops.turboist.core.model

            import kotlinx.serialization.Serializable

            @Serializable
            class Tagged

            @Deprecated("gone")
            class Untagged

            class Bare
            """,
        )

        assertEquals(listOf("ru.tinyops.turboist.core.model.Tagged"), surfaces.map { it.className })
    }

    @Test
    fun `a file with no package declaration yields nothing`() {
        assertTrue(of("@Serializable class Loose").isEmpty())
    }
}
