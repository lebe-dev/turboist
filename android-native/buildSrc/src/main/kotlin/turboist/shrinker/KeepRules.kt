package turboist.shrinker

/**
 * Reads the class patterns out of a shrinker configuration file.
 *
 * Only what is needed to audit coverage is understood: the class name pattern
 * of every directive that preserves the class *itself*. A rule that only keeps
 * members leaves the shrinker free to rename or remove the class around them, so
 * it proves nothing about a class that has to survive under the name it was
 * written with and is deliberately not read as coverage. Member specifications
 * and the non-keep options carry no coverage of their own either, so a rule that
 * is meant to protect a class has to say so in the plain
 * `-keep … class <pattern>` form the audit can read.
 */
object KeepRules {
    private val CLASS_KEYWORDS = setOf("class", "interface", "enum", "@interface")

    /**
     * The directives that preserve the named class itself.
     *
     * `-keepclassmembers` and `-keepclassmembernames` are absent on purpose:
     * they say what to do with a class's members *if* the class survives, and a
     * class nothing else points at does not.
     */
    private val CLASS_PRESERVING = setOf(
        "-keep",
        "-keepnames",
        "-keepclasseswithmembers",
        "-keepclasseswithmembernames",
    )

    /** Modifiers that may sit between the directive and the class keyword. */
    private val MODIFIERS = setOf(
        "public", "final", "abstract", "static", "synthetic", "!public", "!final", "!abstract",
    )

    /**
     * Every class name pattern the file keeps, in the order they appear.
     */
    fun classPatterns(configuration: String): List<String> =
        configuration.lineSequence()
            .map { it.substringBefore('#').trim() }
            .filter { it.startsWith("-keep") }
            .mapNotNull(::classPatternOf)
            .toList()

    private fun classPatternOf(directiveLine: String): String? {
        // `-keep,allowobfuscation class …` — the modifiers after the comma change
        // what may happen to the class, not which directive it is.
        val directive = directiveLine.substringBefore(' ').substringBefore(',')
        if (directive !in CLASS_PRESERVING) return null

        val specification = directiveLine
            .substringAfter(' ', missingDelimiterValue = "")
            .substringBefore('{')
            .trim()
        if (specification.isEmpty()) return null

        val tokens = specification.split(Regex("\\s+"))
        var index = 0
        while (index < tokens.size) {
            val token = tokens[index]
            when {
                // A conditional keep (`-keep @Some.Annotation class …`) still
                // names the classes it protects; the condition only narrows it.
                token.startsWith("@") && token !in CLASS_KEYWORDS -> index++
                token in MODIFIERS -> index++
                token in CLASS_KEYWORDS -> return tokens.getOrNull(index + 1)
                else -> return null
            }
        }
        return null
    }

    /**
     * Whether a pattern names the given class, using the shrinker's own wildcard
     * meanings: `**` spans package separators, `*` and `?` do not.
     */
    fun covers(pattern: String, className: String): Boolean =
        regexOf(pattern).matches(className)

    /** The first pattern that names the class, or `null` when none does. */
    fun coveringPattern(patterns: List<String>, className: String): String? =
        patterns.firstOrNull { covers(it, className) }

    private fun regexOf(pattern: String): Regex {
        val built = StringBuilder("^")
        var index = 0
        while (index < pattern.length) {
            when {
                pattern.startsWith("**", index) -> {
                    built.append(".*")
                    index += 2
                }
                pattern[index] == '*' -> {
                    built.append("[^.]*")
                    index++
                }
                pattern[index] == '?' -> {
                    built.append("[^.]")
                    index++
                }
                else -> {
                    built.append(Regex.escape(pattern[index].toString()))
                    index++
                }
            }
        }
        return Regex(built.append('$').toString())
    }
}
