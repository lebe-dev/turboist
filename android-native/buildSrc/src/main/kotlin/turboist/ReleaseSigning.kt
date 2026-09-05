package turboist

/**
 * The four values a release build needs in order to be signed with the upload
 * key. They are supplied by the machine doing the build — as Gradle properties
 * or as environment variables — and never live in the repository.
 */
data class SigningCredentials(
    val storeFile: String,
    val storePassword: String,
    val keyAlias: String,
    val keyPassword: String,
)

/**
 * Reads the signing credentials out of whatever the build was given.
 *
 * Two outcomes are legitimate and one is not. A machine that supplies none of
 * the four is building for itself, and gets an unsigned release artifact — that
 * is what makes `assembleRelease` runnable on a laptop with no secrets on it. A
 * machine that supplies all four signs with the upload key. A machine that
 * supplies some of them meant to sign and cannot, so the build is stopped: an
 * unsigned artifact that was supposed to be signed is the one failure that would
 * otherwise be discovered at the store's upload form.
 */
object ReleaseSigning {
    const val STORE_FILE: String = "TURBOIST_ANDROID_KEYSTORE"
    const val STORE_PASSWORD: String = "TURBOIST_ANDROID_KEYSTORE_PASSWORD"
    const val KEY_ALIAS: String = "TURBOIST_ANDROID_KEY_ALIAS"
    const val KEY_PASSWORD: String = "TURBOIST_ANDROID_KEY_PASSWORD"

    /** Every name a build machine is asked for, in the order they are documented. */
    val NAMES: List<String> = listOf(STORE_FILE, STORE_PASSWORD, KEY_ALIAS, KEY_PASSWORD)

    /**
     * @param lookup resolves one name to its value, or `null` when the machine
     *   does not supply it. A blank value counts as not supplied.
     */
    fun from(lookup: (String) -> String?): SigningCredentials? {
        val supplied = NAMES.associateWith { name -> lookup(name)?.trim()?.takeIf(String::isNotEmpty) }
        val missing = NAMES.filter { supplied[it] == null }
        if (missing.size == NAMES.size) return null
        check(missing.isEmpty()) {
            // Only the missing names are printed. The supplied ones include two
            // passwords, and a build log is not a place to put them.
            "the release signing credentials are incomplete; missing: ${missing.joinToString(", ")}"
        }
        return SigningCredentials(
            storeFile = supplied.getValue(STORE_FILE)!!,
            storePassword = supplied.getValue(STORE_PASSWORD)!!,
            keyAlias = supplied.getValue(KEY_ALIAS)!!,
            keyPassword = supplied.getValue(KEY_PASSWORD)!!,
        )
    }
}
