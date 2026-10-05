package ru.tinyops.turboist.core.network.mapping

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.longOrNull
import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.HarpoonKind
import ru.tinyops.turboist.core.model.HarpoonRef
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.model.UserSettings
import ru.tinyops.turboist.core.model.UserState
import ru.tinyops.turboist.core.model.normalizeMaxPinned
import ru.tinyops.turboist.core.network.dto.AppSettingsDto
import ru.tinyops.turboist.core.network.dto.AutoLabelRuleDto
import ru.tinyops.turboist.core.network.dto.HarpoonDto
import ru.tinyops.turboist.core.network.dto.ProjectSuggestionRuleDto
import ru.tinyops.turboist.core.network.dto.UserSettingsDto

/**
 * The user's own preferences.
 *
 * Two details are worth stating rather than inferring from the code:
 *
 * - The label ids inside stay **server** ids. This blob travels back to the
 *   server unchanged on the next write, and rewriting the ids to local ones would
 *   corrupt it.
 * - The banner's day part is an empty string for "all day", which is a different
 *   thing from the `none` phase; that is why the domain type spells it as an
 *   absent value.
 *
 * @param harpoon the jump pair, which the preferences payload does not carry —
 *   it has an endpoint of its own — so a caller that has read it passes it in.
 */
fun UserSettingsDto.toUserSettings(harpoon: List<HarpoonRef> = emptyList()): UserSettings =
    UserSettings(
        weeklyUnplannedExcludedLabelIds = weeklyUnplannedExcludedLabelIds,
        bugLabelIds = bugLabelIds,
        locale = locale,
        publicView = publicView,
        bannerText = bannerText,
        bannerPublished = bannerPublished,
        bannerDayPart = DayPart.fromWireOrNull(bannerDayPart),
        calendarEnabled = calendarEnabled,
        calendarHidePastEvents = calendarHidePastEvents,
        troikiEnabled = troikiEnabled,
        maxPinnedTasks = normalizeMaxPinned(maxPinnedTasks),
        maxPinnedProjects = normalizeMaxPinned(maxPinnedProjects),
        harpoon = harpoon,
    )

/** The jump pair. Its ids are server ids for the same reason the settings blob's are. */
fun HarpoonDto.toHarpoon(): List<HarpoonRef> =
    slots.map { HarpoonRef(kind = HarpoonKind.fromWire(it.kind), id = it.id) }

fun AutoLabelRuleDto.toRule(): AutoLabelRule = AutoLabelRule(mask = mask, labelIds = labelIds, ignoreCase = ignoreCase)

fun ProjectSuggestionRuleDto.toRule(): ProjectSuggestionRule =
    ProjectSuggestionRule(mask = mask, projectIds = projectIds, ignoreCase = ignoreCase)

fun AppSettingsDto.toAppSettings(): AppSettings =
    AppSettings(
        autoLabels = autoLabels.map { it.toRule() },
        projectSuggestions = projectSuggestions.map { it.toRule() },
    )

/**
 * The interface-state blob.
 *
 * The server keeps it as an opaque object and merges writes key by key, so
 * reading only the keys this client understands is safe: a key it ignores is left
 * alone rather than overwritten.
 */
fun JsonObject.toUserState(): UserState =
    UserState(activeContextId = (this["activeContextId"] as? JsonPrimitive)?.longOrNull)
