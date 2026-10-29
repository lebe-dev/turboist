package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.quickadd.MAX_PROJECT_SUGGESTIONS
import ru.tinyops.turboist.nativeapp.quickadd.ruleMatches
import ru.tinyops.turboist.nativeapp.settings.RuleEditor
import ru.tinyops.turboist.nativeapp.settings.RuleKind
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState

/**
 * The rules the whole installation obeys, and the label lists the account keeps.
 *
 * The rules are not the user's preferences: they live in a second document with
 * its own endpoints, and they apply to the server rather than to a person.
 *
 * The two rule lists look alike and do opposite things, which is the one thing
 * this section has to make impossible to miss. An **auto-label** rule *acts*: a
 * task whose title matches comes out already carrying the labels, with nobody
 * asked. A **project suggestion** rule only *offers*: the matching projects
 * appear while the user types and reach the task only if the user picks one.
 * Each list therefore carries the sentence that says which of the two it is,
 * and so does the editor.
 */
@Composable
internal fun RulesSection(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    val labelNames = state.nameableLabels.associate { it.serverId to it.name }
    val projectNames = state.nameableProjects.associate { it.serverId to it.title }

    SettingsGroupLabel(stringResource(R.string.settings_autoLabels_heading))
    Hint(R.string.settings_autoLabels_description)
    Hint(R.string.native_settings_ruleAppliedAutomatically)
    SettingsGroup {
        state.app.autoLabels.forEachIndexed { index, rule ->
            row {
                RuleRow(
                    mask = rule.mask,
                    ignoreCase = rule.ignoreCase,
                    targets = rule.labelIds.mapNotNull { labelNames[it] },
                    onEdit = { callbacks.onEditRule(RuleKind.AUTO_LABEL, index) },
                    onDelete = { callbacks.onDeleteRule(RuleKind.AUTO_LABEL, index) },
                )
            }
        }
        if (state.app.autoLabels.isEmpty()) row { SettingsNote(stringResource(R.string.settings_autoLabels_empty)) }
        row { AddRuleRow(R.string.settings_autoLabels_add) { callbacks.onStartNewRule(RuleKind.AUTO_LABEL) } }
    }

    SettingsGroupLabel(stringResource(R.string.settings_projectSuggestions_heading))
    HintText(stringResource(R.string.settings_projectSuggestions_description, MAX_PROJECT_SUGGESTIONS.toString()))
    Hint(R.string.native_settings_ruleOnlySuggested)
    SettingsGroup {
        state.app.projectSuggestions.forEachIndexed { index, rule ->
            row {
                RuleRow(
                    mask = rule.mask,
                    ignoreCase = rule.ignoreCase,
                    targets = rule.projectIds.mapNotNull { projectNames[it] },
                    onEdit = { callbacks.onEditRule(RuleKind.PROJECT_SUGGESTION, index) },
                    onDelete = { callbacks.onDeleteRule(RuleKind.PROJECT_SUGGESTION, index) },
                )
            }
        }
        if (state.app.projectSuggestions.isEmpty()) {
            row { SettingsNote(stringResource(R.string.settings_projectSuggestions_empty)) }
        }
        row {
            AddRuleRow(R.string.settings_projectSuggestions_add) {
                callbacks.onStartNewRule(RuleKind.PROJECT_SUGGESTION)
            }
        }
    }

    LabelListSettings(state, callbacks)
}

/**
 * One rule, read as the sentence it is: this mask, these things.
 *
 * An id that no longer resolves contributes nothing rather than an empty name —
 * the rule is real, the row it names is simply not on this device, and the
 * server skips such an id too.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun RuleRow(
    mask: String,
    ignoreCase: Boolean,
    targets: List<String>,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    ListItem(
        headlineContent = {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Surface(shape = RoundedCornerShape(6.dp), color = MaterialTheme.colorScheme.surfaceContainerHighest) {
                    Text(
                        text = mask,
                        fontFamily = FontFamily.Monospace,
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
                    )
                }
                if (ignoreCase) {
                    Text(
                        text = stringResource(R.string.native_settings_ruleIgnoreCaseMark),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        },
        supportingContent =
            targets.takeIf { it.isNotEmpty() }?.let {
                {
                    FlowRow(
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                        modifier = Modifier.padding(top = 6.dp),
                    ) {
                        for (target in targets) {
                            Surface(
                                shape = RoundedCornerShape(12.dp),
                                color = MaterialTheme.colorScheme.primaryContainer,
                            ) {
                                Text(
                                    text = target,
                                    style = MaterialTheme.typography.labelMedium,
                                    color = MaterialTheme.colorScheme.onPrimaryContainer,
                                    modifier = Modifier.padding(horizontal = 10.dp, vertical = 3.dp),
                                )
                            }
                        }
                    }
                }
            },
        trailingContent = {
            IconButton(onClick = onDelete) {
                Icon(
                    imageVector = Icons.Outlined.Delete,
                    contentDescription = stringResource(R.string.settings_autoLabels_remove),
                )
            }
        },
        colors = groupRowColors(),
        modifier = Modifier.fillMaxWidth().clickable(onClick = onEdit),
    )
}

@Composable
private fun AddRuleRow(
    labelRes: Int,
    onClick: () -> Unit,
) {
    ListItem(
        headlineContent = { Text(stringResource(labelRes), color = MaterialTheme.colorScheme.primary) },
        leadingContent = {
            Icon(imageVector = Icons.Outlined.Add, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
        },
        colors = groupRowColors(),
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick),
    )
}

/**
 * The two lists of labels the user's preferences name.
 *
 * A label that has never reached the server is not offered: the lists travel
 * back to the server as its own ids, and a device's id means nothing there. The
 * label appears here the moment the queue drains.
 */
@Composable
private fun LabelListSettings(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    val labels = state.nameableLabels

    SettingsGroupLabel(stringResource(R.string.settings_weekly_heading))
    Hint(R.string.settings_weekly_description)
    SettingsGroup {
        row {
            LabelChoices(
                labels = labels,
                chosenServerIds = state.user.weeklyUnplannedExcludedLabelIds,
                emptyRes = R.string.settings_weekly_empty,
                onToggle = callbacks.onToggleWeeklyExcludedLabel,
            )
        }
    }

    SettingsGroupLabel(stringResource(R.string.settings_project_bugLabelsHeading))
    Hint(R.string.settings_project_bugLabelsDescription)
    SettingsGroup {
        row {
            LabelChoices(
                labels = labels,
                chosenServerIds = state.user.bugLabelIds,
                emptyRes = R.string.settings_project_bugLabelsEmpty,
                onToggle = callbacks.onToggleBugLabel,
            )
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun LabelChoices(
    labels: List<Label>,
    chosenServerIds: List<Long>,
    emptyRes: Int,
    onToggle: (Long) -> Unit,
) {
    if (labels.isEmpty()) {
        SettingsNote(stringResource(emptyRes))
        return
    }
    FlowRow(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        for (label in labels) {
            val serverId = label.serverId ?: continue
            ChoiceChip(name = label.name, selected = serverId in chosenServerIds, onClick = { onToggle(serverId) })
        }
    }
}

/** A filter chip that shows a tick while it is chosen, so the choice is not carried by colour alone. */
@Composable
private fun ChoiceChip(
    name: String,
    selected: Boolean,
    onClick: () -> Unit,
) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        label = { Text(name) },
        leadingIcon =
            if (selected) {
                {
                    Icon(
                        Icons.Outlined.Check,
                        contentDescription = null,
                        modifier = Modifier.size(FilterChipDefaults.IconSize),
                    )
                }
            } else {
                null
            },
    )
}

/**
 * The editor both rule lists share, raised from the bottom of the screen.
 *
 * One editor rather than two, because the shape is identical — a mask, a set of
 * things, and whether case matters. What differs is what the rule *does*, and
 * that is said in words at the top rather than left for the user to remember
 * from which button they pressed.
 *
 * The title field under the controls is a rehearsal: type a task title and the
 * editor says whether this rule would match it, by the same test quick add
 * applies. Nothing typed there is kept.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
internal fun RuleEditorSheet(
    editor: RuleEditor,
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    val autoLabels = editor.kind == RuleKind.AUTO_LABEL
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var probe by rememberSaveable(editor.kind, editor.index) { mutableStateOf("") }
    val headingRes =
        if (autoLabels) R.string.settings_autoLabels_heading else R.string.settings_projectSuggestions_heading
    val titleRes = if (editor.index == null) R.string.native_settings_ruleNew else R.string.native_settings_ruleEdit
    val maskRes = if (autoLabels) R.string.settings_autoLabels_mask else R.string.settings_projectSuggestions_mask
    val ignoreCaseRes =
        if (autoLabels) R.string.settings_autoLabels_ignoreCase else R.string.settings_projectSuggestions_ignoreCase

    ModalBottomSheet(onDismissRequest = callbacks.onCancelRule, sheetState = sheetState) {
        Column(
            modifier =
                Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 24.dp)
                    .navigationBarsPadding()
                    .padding(bottom = 16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    text =
                        stringResource(
                            headingRes,
                        ),
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.primary,
                )
                Text(
                    text =
                        stringResource(
                            titleRes,
                        ),
                    style = MaterialTheme.typography.headlineSmall,
                )
                Text(
                    text =
                        stringResource(
                            if (autoLabels) {
                                R.string.native_settings_ruleAppliedAutomatically
                            } else {
                                R.string.native_settings_ruleOnlySuggested
                            },
                        ),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            OutlinedTextField(
                value = editor.mask,
                onValueChange = callbacks.onSetRuleMask,
                singleLine = true,
                label = {
                    Text(
                        stringResource(
                            maskRes,
                        ),
                    )
                },
                placeholder = {
                    Text(
                        stringResource(
                            if (autoLabels) {
                                R.string.settings_autoLabels_maskPlaceholder
                            } else {
                                R.string.settings_projectSuggestions_maskPlaceholder
                            },
                        ),
                    )
                },
                modifier = Modifier.fillMaxWidth(),
            )
            SettingsSwitchRow(
                titleRes = ignoreCaseRes,
                checked = editor.ignoreCase,
                onCheckedChange = callbacks.onSetRuleIgnoreCase,
            )
            RuleTargets(editor = editor, state = state, onToggle = callbacks.onToggleRuleTarget)
            OutlinedTextField(
                value = probe,
                onValueChange = { probe = it },
                singleLine = true,
                label = { Text(stringResource(R.string.native_settings_ruleProbe)) },
                modifier = Modifier.fillMaxWidth(),
            )
            if (probe.isNotBlank()) ProbeVerdict(ruleMatches(editor.mask.trim(), editor.ignoreCase, probe))
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
            ) {
                TextButton(onClick = callbacks.onCancelRule) { Text(stringResource(R.string.common_cancel)) }
                Button(onClick = callbacks.onSaveRule, enabled = editor.canSave) {
                    Text(stringResource(R.string.common_save))
                }
            }
        }
    }
}

@Composable
private fun ProbeVerdict(matches: Boolean) {
    val scheme = MaterialTheme.colorScheme
    AssistChip(
        onClick = {},
        enabled = false,
        label = {
            Text(
                stringResource(
                    if (matches) R.string.native_settings_ruleProbeMatch else R.string.native_settings_ruleProbeNoMatch,
                ),
            )
        },
        leadingIcon = {
            Icon(
                imageVector = if (matches) Icons.Outlined.Check else Icons.Outlined.Close,
                contentDescription = null,
                tint = if (matches) scheme.primary else scheme.onSurfaceVariant,
            )
        },
    )
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun RuleTargets(
    editor: RuleEditor,
    state: SettingsUiState,
    onToggle: (Long) -> Unit,
) {
    val offered: List<Pair<Long, String>> =
        when (editor.kind) {
            RuleKind.AUTO_LABEL -> state.nameableLabels.mapNotNull { label -> label.serverId?.let { it to label.name } }
            RuleKind.PROJECT_SUGGESTION ->
                state.nameableProjects.mapNotNull { project -> project.serverId?.let { it to project.title } }
        }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            text =
                stringResource(
                    if (editor.kind == RuleKind.AUTO_LABEL) {
                        R.string.settings_autoLabels_labels
                    } else {
                        R.string.settings_projectSuggestions_projects
                    },
                ),
            style = MaterialTheme.typography.labelLarge,
        )
        if (offered.isEmpty()) {
            Text(
                text =
                    stringResource(
                        if (editor.kind == RuleKind.AUTO_LABEL) {
                            R.string.settings_autoLabels_noLabelsAvailable
                        } else {
                            R.string.settings_projectSuggestions_noProjectsAvailable
                        },
                    ),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            return@Column
        }
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            for ((serverId, name) in offered) {
                ChoiceChip(name = name, selected = serverId in editor.targetServerIds, onClick = { onToggle(serverId) })
            }
        }
    }
}
