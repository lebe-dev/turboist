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
 * Server-wide settings, one row for the whole installation — distinct from
 * [UserSettings], which is the user's own preferences. The two are never merged:
 * they have different owners and different write paths.
 */
data class AppSettings(
    val autoLabels: List<AutoLabelRule> = emptyList(),
    val projectSuggestions: List<ProjectSuggestionRule> = emptyList(),
)
