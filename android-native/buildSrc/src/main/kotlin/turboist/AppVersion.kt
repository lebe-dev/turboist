package turboist

/**
 * Derivation of the Android version fields from the repository-root `VERSION`
 * file, so the native client can never drift from the server it talks to.
 *
 * The file holds a semantic version, optionally with a pre-release or build
 * suffix (`1.17.0`, `1.17.0-dev`, `1.17.0+abc1234`). The suffix identifies a
 * build, not a release ordering, so it is dropped before the numeric code is
 * computed.
 */
object AppVersion {
    private const val MINOR_SCALE: Int = 1_000
    private const val MAJOR_SCALE: Int = 1_000_000

    /** Largest value any single component may take, given the scale factors above. */
    const val MAX_COMPONENT: Int = 999

    private val SEMVER = Regex("""^(\d+)\.(\d+)\.(\d+)$""")

    /**
     * Strips a pre-release/build suffix and any surrounding whitespace, leaving
     * a bare `major.minor.patch` string.
     */
    fun normalize(raw: String): String {
        val trimmed = raw.trim()
        require(trimmed.isNotEmpty()) { "VERSION is empty" }
        return trimmed.substringBefore('-').substringBefore('+').trim()
    }

    /**
     * Maps `major.minor.patch` onto a single ascending integer, so that a newer
     * release always yields a larger code than every release before it.
     *
     * Each component is capped at [MAX_COMPONENT]; a wider component would carry
     * into its neighbour and break that ordering, so it is rejected instead.
     */
    fun versionCode(raw: String): Int {
        val normalized = normalize(raw)
        val match = SEMVER.matchEntire(normalized)
            ?: throw IllegalArgumentException(
                "VERSION must be major.minor.patch with an optional suffix, got: ${raw.trim()}",
            )
        val (major, minor, patch) = match.destructured
        val parts = listOf(major, minor, patch).map { part ->
            val value = part.toIntOrNull()
                ?: throw IllegalArgumentException("version component out of range in: ${raw.trim()}")
            require(value <= MAX_COMPONENT) {
                "version component $value exceeds $MAX_COMPONENT in: ${raw.trim()}"
            }
            value
        }
        return parts[0] * MAJOR_SCALE + parts[1] * MINOR_SCALE + parts[2]
    }
}

/**
 * Composition of the version string a build shows to the user.
 *
 * A build made from the repository state alone shows the bare release version.
 * A build a person is handed — a release artifact, or one sideloaded from a
 * working tree — additionally carries the commit it was built from, so a bug
 * report identifies the exact code that produced it. The suffix is a build
 * identifier and never affects ordering, which is why the numeric code above
 * drops it.
 */
fun AppVersion.buildVersionName(
    raw: String,
    buildStamp: String?,
): String {
    val version = normalize(raw)
    val stamp = buildStamp?.trim().orEmpty()
    if (stamp.isEmpty()) return version
    require(stamp.all { it.isLetterOrDigit() || it == '.' }) {
        "the build stamp may only hold letters, digits and dots, got: $stamp"
    }
    return "$version+$stamp"
}
