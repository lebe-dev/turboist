package turboist.shrinker

import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Audits the shipped shrinker configuration against the sources it has to
 * protect.
 *
 * A minified build removes and renames everything nothing points at, and the
 * classes something finds by name at runtime are exactly the ones nothing points
 * at in a way the shrinker can see. When one of those is missed the build still
 * succeeds and the failure arrives as a crash on a user's phone, which is the
 * worst possible place to learn about it. So the surfaces are enumerated from
 * the sources on every run and held against the rules, and adding a wire type,
 * an endpoint interface or a database without a rule for it fails here.
 *
 * The set of modules to audit is read from the build's own module list rather
 * than repeated here: a list kept by hand would let a new module ship with no
 * rules at all and still leave this check green, which is the one outcome it
 * exists to prevent.
 */
class ShrinkerCoverageTest {
    /** One module: its Gradle path, where its sources are, and the rules files that ship with it. */
    private data class Module(
        val name: String,
        val sourceRoots: List<File>,
        val rules: List<File>,
    )

    private companion object {
        /** `include(":core:model")` in the build's module list. */
        val INCLUDE = Regex("""include\(\s*"(:[A-Za-z0-9_\-:]+)"\s*\)""")

        /** Source-set directories holding code that is not compiled into a release build. */
        val TEST_SOURCE_SETS = Regex("""^(test|androidTest)""")
    }

    private val root = File("..")

    private val modules: List<Module> = declaredModules()

    /**
     * Every pattern the shrinker is given. A library module's rules travel with
     * it into the app, so what protects a class is the union of the files rather
     * than the one beside it.
     */
    private val patterns: List<String> =
        modules.flatMap { it.rules }.flatMap { KeepRules.classPatterns(it.readText()) }

    private val surfaces: Map<Module, List<ReflectionSurfaces.Surface>> =
        modules.associateWith(::surfacesOf)

    @Test
    fun `every class the runtime looks up by name survives the shrinker`() {
        val uncovered = surfaces.flatMap { (module, found) ->
            found.filter { KeepRules.coveringPattern(patterns, it.className) == null }
                .map { "${module.name}: ${it.className} (${it.reason})" }
        }

        assertTrue(
            uncovered.isEmpty(),
            buildString {
                appendLine("no shrinker rule keeps these classes, so a minified build may rename or delete them:")
                uncovered.sorted().forEach { appendLine("  $it") }
                appendLine("add a `-keep` for each to the rules file of the module it belongs to.")
            },
        )
    }

    @Test
    fun `the audit is still looking at all three reflection surfaces`() {
        // A refactor that moves the wire types, renames the endpoint package or
        // replaces the database class would leave the enumerator finding nothing
        // and the audit passing on an empty set — a green check that proves the
        // opposite of what it claims.
        val byReason = surfaces.values.flatten().groupingBy { it.reason }.eachCount()

        ReflectionSurfaces.Reason.values().forEach { reason ->
            assertTrue(
                (byReason[reason] ?: 0) > 0,
                "the sources no longer declare anything the shrinker must keep for $reason; " +
                    "either the enumerator stopped recognising it or the surface really is gone",
            )
        }
    }

    @Test
    fun `the domain module stays free of anything the runtime looks up by name`() {
        // The domain types carry no serialization, persistence or HTTP
        // annotations by design; anything found here means a layer above has
        // leaked into them.
        val model = modules.first { it.name == "core:model" }
        assertEquals(
            emptyList(),
            surfaces.getValue(model).map { it.className },
            "the domain types must stay free of serialization, persistence and HTTP annotations",
        )
    }

    @Test
    fun `every module in the tree is audited`() {
        // Two independent sources of truth: what the build declares, and what is
        // on disk. A module directory that carries its own build script but is
        // not audited would be shipped with nobody checking its rules.
        val onDisk = root.listFiles().orEmpty()
            .filter { it.isDirectory && it.name != "buildSrc" }
            .flatMap { listOf(it) + (it.listFiles().orEmpty().filter(File::isDirectory)) }
            .filter { File(it, "build.gradle.kts").isFile }
            .map { it.relativeTo(root).path.replace(File.separatorChar, ':') }
            .sorted()

        assertEquals(
            onDisk,
            modules.map { it.name }.sorted(),
            "these module directories and the build's module list disagree; a module missing from " +
                "the list ships with its shrinker rules unchecked",
        )
    }

    @Test
    fun `the enumerator can read every production source it is pointed at`() {
        // The enumerator parses Kotlin. A production source written in another
        // language would contribute surfaces it cannot see, and the audit would
        // stay green while the shrinker deleted them.
        val unreadable = modules.flatMap { module ->
            module.sourceRoots.flatMap { it.walkTopDown().filter(File::isFile) }
                .filter { it.extension != "kt" && it.extension != "kts" }
                .map { "${module.name}: ${it.relativeTo(root).path}" }
        }

        assertTrue(
            unreadable.isEmpty(),
            buildString {
                appendLine("the reflection-surface enumerator reads Kotlin, and these production sources are not Kotlin:")
                unreadable.sorted().forEach { appendLine("  $it") }
                appendLine("teach the enumerator to read them, or the shrinker audit does not cover them.")
            },
        )
    }

    // --- what to audit --------------------------------------------------------

    private fun declaredModules(): List<Module> {
        val settings = File(root, "settings.gradle.kts")
        assertTrue(settings.isFile, "the build's module list is not where it is expected: ${settings.absolutePath}")

        val paths = INCLUDE.findAll(settings.readText()).map { it.groupValues[1].removePrefix(":") }.toList()
        assertTrue(paths.isNotEmpty(), "no modules found in ${settings.absolutePath}; the audit would pass vacuously")

        return paths.map { path ->
            val directory = File(root, path.replace(':', File.separatorChar))
            assertTrue(directory.isDirectory, "module $path is declared by the build but has no directory")
            Module(
                name = path,
                sourceRoots = productionSourceRoots(directory),
                rules = rulesOf(directory),
            )
        }
    }

    /**
     * Every source root a release build compiles: each source set that is not a
     * test one, so a flavour or a build type added later is audited without this
     * file changing.
     */
    private fun productionSourceRoots(directory: File): List<File> =
        File(directory, "src").listFiles().orEmpty()
            .filter { it.isDirectory && !TEST_SOURCE_SETS.containsMatchIn(it.name) }
            .flatMap { sourceSet -> listOf(File(sourceSet, "kotlin"), File(sourceSet, "java")) }
            .filter { it.isDirectory }

    private fun rulesOf(directory: File): List<File> =
        listOf("consumer-rules.pro", "proguard-rules.pro").map { File(directory, it) }.filter { it.isFile }

    private fun surfacesOf(module: Module): List<ReflectionSurfaces.Surface> =
        module.sourceRoots.flatMap { sourceRoot ->
            sourceRoot.walkTopDown()
                .filter { it.isFile && it.extension == "kt" }
                .flatMap { ReflectionSurfaces.of(it.readText()) }
                .toList()
        }
}
