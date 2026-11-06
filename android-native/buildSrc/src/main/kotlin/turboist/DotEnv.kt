package turboist

/**
 * The repository's `.env` file, as a build reads it.
 *
 * The recipes in the Justfile already load this file into the environment, but a
 * build started by hand or from an IDE does not go through them. Reading the
 * file here means both kinds of build see the same values, so an artifact cannot
 * silently lose its configuration because of how Gradle happened to be started.
 */
object DotEnv {
    private val NAME = Regex("[A-Za-z_][A-Za-z0-9_]*")

    /** The name/value pairs in [text]; lines that are not assignments are ignored. */
    fun parse(text: String): Map<String, String> =
        text
            .lineSequence()
            .mapNotNull(::assignment)
            .toMap()

    /**
     * Resolves a name the way a shell-started build would: the real environment
     * wins, and the file fills in what it leaves out. A blank value counts as not
     * supplied, so an empty placeholder in either place cannot hide a real one.
     */
    fun lookup(
        environment: (String) -> String?,
        file: Map<String, String>,
    ): (String) -> String? =
        { name ->
            environment(name)?.takeIf(String::isNotBlank) ?: file[name]?.takeIf(String::isNotBlank)
        }

    private fun assignment(line: String): Pair<String, String>? {
        val trimmed = line.trim().removePrefix("export ").trimStart()
        if (trimmed.isEmpty() || trimmed.startsWith("#")) return null
        val separator = trimmed.indexOf('=')
        if (separator <= 0) return null
        val name = trimmed.substring(0, separator).trim()
        if (!NAME.matches(name)) return null
        return name to valueOf(trimmed.substring(separator + 1).trim())
    }

    private fun valueOf(raw: String): String {
        val quote = raw.firstOrNull()
        if (quote == '"' || quote == '\'') {
            val end = raw.indexOf(quote, startIndex = 1)
            if (end > 0) return raw.substring(1, end)
        }
        // A hash only starts a comment after whitespace: a DSN or a token may
        // legitimately contain one.
        return raw.split(Regex("\\s#"), limit = 2).first().trim()
    }
}
