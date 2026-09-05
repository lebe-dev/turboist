package turboist.shrinker

/**
 * The declarations in a Kotlin source file that something finds by name at
 * runtime rather than by a compiled reference.
 *
 * A shrinker is free to rename or delete anything nothing points at, and these
 * are exactly the classes nothing points at in a way it can see. Left
 * unprotected they disappear from a minified build, and the failure surfaces as
 * a crash on a user's phone rather than as a broken build — which is why they
 * are enumerated from the sources instead of listed by hand.
 */
object ReflectionSurfaces {
    /** Why a class has to survive the shrinker under the name it was written with. */
    enum class Reason {
        /** Serializer generation and polymorphic discriminators are keyed on the class. */
        SERIALIZATION,

        /** The HTTP client builds an implementation from the interface's annotations. */
        RETROFIT,

        /** The database implementation is looked up by appending a suffix to the class name. */
        ROOM,
    }

    /** One class that has to keep its name, and the reason it has to. */
    data class Surface(
        val className: String,
        val reason: Reason,
    )

    private val PACKAGE = Regex("""^package\s+([\w.]+)""")

    private const val MODIFIERS =
        "public|internal|private|protected|abstract|final|open|sealed|data|value|inline|enum|annotation|companion|expect|actual"

    private val DECLARATION =
        Regex("""^(\s*)(?:(?:$MODIFIERS)\s+)*(class|interface|object)\s+([A-Za-z_][A-Za-z0-9_]*)""")

    private val ANNOTATION = Regex("""^@([A-Za-z_][A-Za-z0-9_]*)""")

    /** Every surface declared in one Kotlin source file. */
    fun of(source: String): List<Surface> {
        val packageName = source.lineSequence()
            .mapNotNull { PACKAGE.find(it.trim())?.groupValues?.get(1) }
            .firstOrNull()
            ?: return emptyList()
        // Retrofit is recognised per file: an interface in a file that imports the
        // HTTP annotations is a call description, and there is nothing else those
        // imports could be there for.
        val declaresHttpCalls = source.contains("import retrofit2.http.")

        val surfaces = mutableListOf<Surface>()
        val enclosing = ArrayDeque<Pair<Int, String>>()
        var annotations = mutableSetOf<String>()
        // How deep the current annotation's argument list is, so a multi-line
        // one is read as one annotation rather than as a run that ended.
        var openBrackets = 0

        source.lineSequence().forEach { line ->
            val trimmed = line.trim()
            if (openBrackets > 0) {
                // Still inside an annotation's argument list. It can run over
                // many lines — a database lists every table it holds — and none
                // of those lines ends the run of annotations.
                openBrackets += bracketBalance(trimmed)
                return@forEach
            }
            val declaration = DECLARATION.find(line)
            when {
                declaration != null -> {
                    val indent = declaration.groupValues[1].length
                    val keyword = declaration.groupValues[2]
                    val name = declaration.groupValues[3]
                    while (enclosing.isNotEmpty() && enclosing.last().first >= indent) {
                        enclosing.removeLast()
                    }
                    val nested = (enclosing.map { it.second } + name).joinToString("$")
                    val className = "$packageName.$nested"
                    if ("Serializable" in annotations) {
                        surfaces += Surface(className, Reason.SERIALIZATION)
                    }
                    if ("Database" in annotations) {
                        // Room writes the implementation beside the class and finds
                        // it by name, so the generated class is what has to survive.
                        surfaces += Surface(className + "_Impl", Reason.ROOM)
                    }
                    if (declaresHttpCalls && keyword == "interface") {
                        surfaces += Surface(className, Reason.RETROFIT)
                    }
                    enclosing.addLast(indent to name)
                    annotations = mutableSetOf()
                }

                ANNOTATION.find(trimmed) != null -> {
                    annotations += ANNOTATION.find(trimmed)!!.groupValues[1]
                    openBrackets = bracketBalance(trimmed).coerceAtLeast(0)
                }

                trimmed.isEmpty() || isComment(trimmed) -> Unit

                // Anything else ends the run of annotations: they only ever
                // attach to the declaration that immediately follows them.
                else -> annotations = mutableSetOf()
            }
        }
        return surfaces
    }

    /** How many brackets a line opens and does not close again. */
    private fun bracketBalance(line: String): Int =
        line.count { it == '(' || it == '[' } - line.count { it == ')' || it == ']' }

    private fun isComment(trimmed: String): Boolean =
        trimmed.startsWith("//") || trimmed.startsWith("/*") || trimmed.startsWith("*")
}
