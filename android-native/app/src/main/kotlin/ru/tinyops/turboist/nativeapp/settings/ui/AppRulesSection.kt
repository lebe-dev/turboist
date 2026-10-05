package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.quickadd.MAX_PROJECT_SUGGESTIONS
import ru.tinyops.turboist.nativeapp.settings.RuleEditor
import ru.tinyops.turboist.nativeapp.settings.RuleKind
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState
import ru.tinyops.turboist.nativeapp.tasks.ui.ChoiceRow

/**
 * The rules the whole installation obeys.
 *
 * These are not the user's preferences and are deliberately drawn apart from
 * them: they live in a second document with its own endpoints, and they apply to
 * the server rather than to a person.
 *
 * The two lists look alike and do opposite things, which is the one thing this
 * section has to make impossible to miss. An **auto-label** rule *acts*: a task
 * whose title matches comes out already carrying the labels, with nobody asked.
 * A **project suggestion** rule only *offers*: the matching projects appear while
 * the user types and reach the task only if the user picks one. Each list
 * therefore carries the sentence that says which of the two it is, and so does
 * the editor.
 */
@Composable
internal fun AppRulesSection(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    SettingsSectionHeading(R.string.settings_autoLabels_heading, R.string.settings_autoLabels_description)
    Hint(R.string.native_settings_ruleAppliedAutomatically)
    if (state.app.autoLabels.isEmpty()) {
        Hint(R.string.settings_autoLabels_empty)
    } else {
        state.app.autoLabels.forEachIndexed { index, rule ->
            RuleRow(
                mask = rule.mask,
                targets = namesOf(rule.labelIds, state.nameableLabels.associate { it.serverId to it.name }),
                onEdit = { callbacks.onEditRule(RuleKind.AUTO_LABEL, index) },
                onDelete = { callbacks.onDeleteRule(RuleKind.AUTO_LABEL, index) },
            )
        }
    }
    AddRuleButton(R.string.settings_autoLabels_add) { callbacks.onStartNewRule(RuleKind.AUTO_LABEL) }

    SettingsSectionHeading(R.string.settings_projectSuggestions_heading)
    HintText(stringResource(R.string.settings_projectSuggestions_description, MAX_PROJECT_SUGGESTIONS.toString()))
    Hint(R.string.native_settings_ruleOnlySuggested)
    if (state.app.projectSuggestions.isEmpty()) {
        Hint(R.string.settings_projectSuggestions_empty)
    } else {
        state.app.projectSuggestions.forEachIndexed { index, rule ->
            RuleRow(
                mask = rule.mask,
                targets = namesOf(rule.projectIds, state.nameableProjects.associate { it.serverId to it.title }),
                onEdit = { callbacks.onEditRule(RuleKind.PROJECT_SUGGESTION, index) },
                onDelete = { callbacks.onDeleteRule(RuleKind.PROJECT_SUGGESTION, index) },
            )
        }
    }
    AddRuleButton(R.string.settings_projectSuggestions_add) {
        callbacks.onStartNewRule(RuleKind.PROJECT_SUGGESTION)
    }
}

/**
 * One rule, read as the sentence it is: this mask, these things.
 *
 * An id that no longer resolves contributes nothing rather than an empty name —
 * the rule is real, the row it names is simply not on this device, and the
 * server skips such an id too.
 */
@Composable
private fun RuleRow(
    mask: String,
    targets: String,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    ListItem(
        headlineContent = { Text(mask) },
        supportingContent = { Text(targets) },
        trailingContent = {
            IconButton(onClick = onDelete) {
                Icon(
                    imageVector = Icons.Outlined.Delete,
                    contentDescription = stringResource(R.string.settings_autoLabels_remove),
                )
            }
        },
        modifier = Modifier.fillMaxWidth().clickable(onClick = onEdit),
    )
}

@Composable
private fun AddRuleButton(
    labelRes: Int,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
        horizontalArrangement = Arrangement.Start,
    ) {
        TextButton(onClick = onClick) { Text(stringResource(labelRes)) }
    }
}

/**
 * The editor both rule lists share.
 *
 * One dialog rather than two, because the shape is identical — a mask, a set of
 * things, and whether case matters. What differs is what the rule *does*, and
 * that is said in words at the top rather than left for the user to remember
 * from which button they pressed.
 */
@Composable
internal fun RuleEditorDialog(
    editor: RuleEditor,
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    val autoLabels = editor.kind == RuleKind.AUTO_LABEL
    AlertDialog(
        onDismissRequest = callbacks.onCancelRule,
        title = {
            Text(
                stringResource(
                    if (autoLabels) {
                        R.string.settings_autoLabels_heading
                    } else {
                        R.string.settings_projectSuggestions_heading
                    },
                ),
            )
        },
        text = {
            Column(modifier = Modifier.verticalScroll(rememberScrollState())) {
                Text(
                    stringResource(
                        if (autoLabels) {
                            R.string.native_settings_ruleAppliedAutomatically
                        } else {
                            R.string.native_settings_ruleOnlySuggested
                        },
                    ),
                )
                OutlinedTextField(
                    value = editor.mask,
                    onValueChange = callbacks.onSetRuleMask,
                    singleLine = true,
                    label = {
                        Text(
                            stringResource(
                                if (autoLabels) {
                                    R.string.settings_autoLabels_mask
                                } else {
                                    R.string.settings_projectSuggestions_mask
                                },
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
                    modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp),
                )
                RuleTargets(editor = editor, state = state, onToggle = callbacks.onToggleRuleTarget)
                SettingsSwitchRow(
                    titleRes =
                        if (autoLabels) {
                            R.string.settings_autoLabels_ignoreCase
                        } else {
                            R.string.settings_projectSuggestions_ignoreCase
                        },
                    checked = editor.ignoreCase,
                    onCheckedChange = callbacks.onSetRuleIgnoreCase,
                )
            }
        },
        confirmButton = {
            TextButton(onClick = callbacks.onSaveRule, enabled = editor.canSave) {
                Text(stringResource(R.string.common_save))
            }
        },
        dismissButton = {
            TextButton(onClick = callbacks.onCancelRule) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

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
    if (offered.isEmpty()) {
        Hint(
            if (editor.kind == RuleKind.AUTO_LABEL) {
                R.string.settings_autoLabels_noLabelsAvailable
            } else {
                R.string.settings_projectSuggestions_noProjectsAvailable
            },
        )
        return
    }
    ChoiceRow(
        label =
            stringResource(
                if (editor.kind == RuleKind.AUTO_LABEL) {
                    R.string.settings_autoLabels_labels
                } else {
                    R.string.settings_projectSuggestions_projects
                },
            ),
    ) {
        for ((serverId, name) in offered) {
            FilterChip(
                selected = serverId in editor.targetServerIds,
                onClick = { onToggle(serverId) },
                label = { Text(name) },
            )
        }
    }
}

/** The names a rule's ids resolve to on this device, as one readable line. */
private fun namesOf(
    serverIds: List<Long>,
    known: Map<Long?, String>,
): String = serverIds.mapNotNull { known[it] }.joinToString(", ")
