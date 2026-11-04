package ru.tinyops.turboist.core.model

/**
 * A rule that attaches labels to a task whose title contains [mask]. Labels are
 * never created by a rule: an id that no longer resolves is skipped.
 *
 * @property labelIds **server** label ids — the rules blob is written by the
 *   server and echoed back unchanged.
 */
data class AutoLabelRule(
    val mask: String,
    val labelIds: List<Long> = emptyList(),
    val ignoreCase: Boolean = false,
)

/**
 * A rule that offers projects for a task whose title contains [mask]. Unlike an
 * auto-label rule nothing is applied: the matches only become suggestions the
 * user can accept while typing.
 *
 * @property projectIds **server** project ids, for the same reason as
 *   [AutoLabelRule.labelIds].
 */
data class ProjectSuggestionRule(
    val mask: String,
    val projectIds: List<Long> = emptyList(),
    val ignoreCase: Boolean = false,
)

/**
 * A title template for the links of one domain and its subdomains.
 *
 * @property domain a bare domain such as `youtube.com`, as the server stores it.
 * @property template the title, with `{title}` standing for the page's own.
 */
data class LinkTitleRule(
    val domain: String,
    val template: String,
)

/**
 * Whether the server names tasks that were created as a bare `https://` link.
 *
 * The work happens on the server once the task reaches it, so a task created
 * offline is named after it syncs; this device only stores and edits the switch
 * and the templates.
 *
 * @property template the default title template; empty means the server's
 *   built-in one for the user's language.
 */
data class LinkTitlesSettings(
    val enabled: Boolean = false,
    val template: String = "",
    val rules: List<LinkTitleRule> = emptyList(),
)

/** What stands for the page title in a template. */
const val LINK_TITLE_PLACEHOLDER: String = "{title}"

/** The longest template the server accepts, in characters. */
const val MAX_LINK_TITLE_TEMPLATE: Int = 200

/**
 * The template the server falls back to when none is set, by UI language. Keep
 * in step with internal/service/linktitle/template.go.
 */
fun defaultLinkTitleTemplate(locale: String): String =
    when (locale.lowercase().substringBefore('-').substringBefore('_')) {
        "ru" -> "Статья «{title}»"
        else -> "Article “{title}”"
    }

/** Whether the server would accept [template] for a rule (it must name the page title). */
fun isValidLinkTitleTemplate(template: String): Boolean {
    val t = template.trim()
    return LINK_TITLE_PLACEHOLDER in t && t.codePointCount(0, t.length) <= MAX_LINK_TITLE_TEMPLATE
}

/**
 * The bare domain a rule matches by, from what people paste —
 * `https://www.YouTube.com/watch` becomes `youtube.com`. The server normalises
 * the same way; doing it here too keeps the list right while the write waits.
 */
fun normalizeLinkTitleDomain(input: String): String {
    var d = input.trim().lowercase()
    for (scheme in listOf("https://", "http://")) d = d.removePrefix(scheme)
    d = d.substringBefore('/').substringBefore('?').substringBefore('#')
    return d.removePrefix("*.").removePrefix("www.").removeSuffix(".")
}

/**
 * Server-wide settings, one row for the whole installation — distinct from
 * [UserSettings], which is the user's own preferences. The two are never merged:
 * they have different owners and different write paths.
 */
data class AppSettings(
    val autoLabels: List<AutoLabelRule> = emptyList(),
    val projectSuggestions: List<ProjectSuggestionRule> = emptyList(),
    val linkTitles: LinkTitlesSettings = LinkTitlesSettings(),
)
