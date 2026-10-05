package ru.tinyops.turboist.nativeapp.quickadd

import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectSuggestionRule

/** How many projects are ever offered for one task title. */
const val MAX_PROJECT_SUGGESTIONS: Int = 3

/**
 * The two title rules the installation carries, read on the device.
 *
 * Both are a mask looked for inside a task title, and there the resemblance
 * ends. **Auto-labels are applied**: a matching rule attaches its labels to the
 * task whether or not anybody looked at it, here and again on the server.
 * **Project suggestions are offered and never applied**: a match only puts a
 * chip in front of the user, and the task goes wherever they then say. Keeping
 * that difference is the whole reason the two are matched by separate functions
 * with separate names rather than by one that takes a flag.
 *
 * Both rule kinds name their targets by **server** id, because the rules
 * document belongs to the server and travels back to it unchanged. A rule naming
 * something this device has never replicated contributes nothing rather than
 * failing: the server applies the same rules authoritatively a moment later.
 *
 * A rule with an empty mask matches nothing. It would otherwise match every
 * title, including the empty one, which is never what half-finished settings
 * mean.
 */
private fun ruleMatches(
    mask: String,
    ignoreCase: Boolean,
    title: String,
): Boolean {
    if (mask.isEmpty()) return false
    return title.contains(mask, ignoreCase = ignoreCase)
}

/**
 * The projects offered for a title.
 *
 * Every rule whose mask occurs in the title contributes its projects; the union
 * is de-duplicated, stripped of finished, unknown and already-chosen projects,
 * sorted by title regardless of case and capped at [limit]. Nothing here is
 * applied — the result is a row of chips, and the task moves only if one is
 * tapped.
 *
 * [excludeLocalIds] is how the picker keeps from offering the project the draft
 * already points at: suggesting where the task is going is noise.
 */
fun matchProjectSuggestions(
    rules: List<ProjectSuggestionRule>,
    title: String,
    projects: List<Project>,
    excludeLocalIds: Set<Long> = emptySet(),
    limit: Int = MAX_PROJECT_SUGGESTIONS,
): List<Project> {
    if (limit <= 0) return emptyList()
    val matched = LinkedHashSet<Long>()
    for (rule in rules) {
        if (!ruleMatches(rule.mask, rule.ignoreCase, title)) continue
        matched += rule.projectIds
    }
    if (matched.isEmpty()) return emptyList()
    return projects
        .filter { project ->
            val serverId = project.serverId ?: return@filter false
            serverId in matched &&
                project.localId !in excludeLocalIds &&
                project.status != ProjectStatus.COMPLETED
        }.sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER, Project::title))
        .take(limit)
}

/**
 * The labels a title will pick up by itself, named.
 *
 * This is the same rule the write path applies when the task is created, run
 * again for the sake of showing it: the user gets to see what is about to be
 * attached, and to take one off before it is. [exclude] holds the names already
 * chosen by hand and the ones the user has just rejected, so neither is offered
 * back to them.
 *
 * Order follows the rules and then the ids inside each rule, so the chips do not
 * reshuffle as more of the title is typed.
 */
fun matchAutoLabelNames(
    rules: List<AutoLabelRule>,
    title: String,
    labels: List<Label>,
    exclude: Set<String> = emptySet(),
): List<String> {
    if (rules.isEmpty()) return emptyList()
    val nameByServerId = labels.mapNotNull { label -> label.serverId?.let { it to label.name } }.toMap()
    val matched = LinkedHashSet<String>()
    for (rule in rules) {
        if (!ruleMatches(rule.mask, rule.ignoreCase, title)) continue
        for (serverId in rule.labelIds) {
            val name = nameByServerId[serverId] ?: continue
            if (name in exclude) continue
            matched += name
        }
    }
    return matched.toList()
}

/** Convenience for the two calls a capture surface makes against one settings document. */
fun AppSettings.suggestedProjects(
    title: String,
    projects: List<Project>,
    excludeLocalIds: Set<Long> = emptySet(),
): List<Project> = matchProjectSuggestions(projectSuggestions, title, projects, excludeLocalIds)

/** Convenience twin of [suggestedProjects] for the labels that will be attached. */
fun AppSettings.autoLabelNames(
    title: String,
    labels: List<Label>,
    exclude: Set<String> = emptySet(),
): List<String> = matchAutoLabelNames(autoLabels, title, labels, exclude)
