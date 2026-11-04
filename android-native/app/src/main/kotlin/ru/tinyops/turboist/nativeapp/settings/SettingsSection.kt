package ru.tinyops.turboist.nativeapp.settings

/**
 * The sections settings is divided into, in the order the top level lists them.
 *
 * Each is a screen of its own, one step below settings. They are grouped by what
 * the user is trying to do rather than by which store a value lives in: the
 * theme sits beside the language because both are "how the app looks", even
 * though one stays on this phone and the other follows the account — the row
 * says which, so the grouping does not have to.
 */
enum class SettingsSection {
    /** Language, theme and Public view. */
    GENERAL,

    /** The Today banner, the drawer's pinning caps, the daily plan and calendars. */
    WORKSPACE,

    /** The installation's two rule lists and the two label lists. */
    RULES,

    /** How the account is signed into, and what else can reach it. */
    SECURITY,

    /** Syncing, the server, and the two actions that empty the device. */
    DEVICE,

    /** The build, and the documents that govern using it. */
    ABOUT,
}

/**
 * One setting as a search can find it.
 *
 * Everything is already in words — the screen resolves its resources before it
 * builds the list — so matching is a question about strings and nothing else.
 *
 * @property title the setting's own name, as its row shows it.
 * @property section where tapping the result leads.
 * @property sectionTitle the section's name, shown under the result and matched too.
 * @property keywords words people reach for that the name does not use.
 */
data class SettingsSearchEntry(
    val title: String,
    val section: SettingsSection,
    val sectionTitle: String,
    val keywords: String = "",
)

/**
 * The entries that match what was typed, in the order they were given.
 *
 * Every word typed has to be found somewhere in the entry, so a second word
 * narrows the list rather than widening it. Nothing typed matches nothing: the
 * sections are the answer to an empty search, not every setting at once.
 */
fun matchSettings(
    query: String,
    entries: List<SettingsSearchEntry>,
): List<SettingsSearchEntry> {
    val words = query.lowercase().split(' ').filter { it.isNotBlank() }
    if (words.isEmpty()) return emptyList()
    return entries.filter { entry ->
        val haystack = "${entry.title} ${entry.sectionTitle} ${entry.keywords}".lowercase()
        words.all { it in haystack }
    }
}
