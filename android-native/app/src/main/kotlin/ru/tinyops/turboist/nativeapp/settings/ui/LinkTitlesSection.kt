package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.LINK_TITLE_PLACEHOLDER
import ru.tinyops.turboist.core.model.defaultLinkTitleTemplate
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.LinkRuleEditor
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState

/** What stands for the link's domain in a template, as the hint names it. */
private const val DOMAIN_PLACEHOLDER = "{domain}"

/**
 * Link titles: a task created as a bare https:// link is named by the server
 * after the page it points to.
 *
 * Everything here is the server's to act on — the device only stores the
 * switch and the templates — so a task created offline is named once it syncs,
 * and the section says so in its description.
 */
@Composable
internal fun LinkTitlesSettings(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    val settings = state.app.linkTitles
    val fallback = defaultLinkTitleTemplate(state.user.locale.ifEmpty { currentLanguage() })

    SettingsGroupLabel(stringResource(R.string.settings_linkTitles_heading))
    Hint(R.string.settings_linkTitles_description)
    SettingsGroup {
        row {
            SettingsSwitchRow(
                titleRes = R.string.settings_linkTitles_enabled,
                checked = settings.enabled,
                onCheckedChange = callbacks.onSetLinkTitlesEnabled,
            )
        }
        if (settings.enabled) {
            row {
                Column(
                    modifier = Modifier.fillMaxWidth().padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    OutlinedTextField(
                        value = state.linkTemplateText,
                        onValueChange = callbacks.onEditLinkTemplate,
                        singleLine = true,
                        label = { Text(stringResource(R.string.settings_linkTitles_template)) },
                        placeholder = { Text(fallback) },
                        supportingText = {
                            Text(
                                stringResource(
                                    R.string.settings_linkTitles_templateHint,
                                    LINK_TITLE_PLACEHOLDER,
                                    DOMAIN_PLACEHOLDER,
                                    fallback,
                                ),
                            )
                        },
                        modifier = Modifier.fillMaxWidth(),
                    )
                    // Typed rather than chosen, so it saves on a button like the
                    // banner text: a write per keystroke would queue a request per
                    // character and hand the server half a template.
                    AnimatedVisibility(visible = state.linkTemplateEdited, modifier = Modifier.align(Alignment.End)) {
                        Button(onClick = callbacks.onSaveLinkTemplate) { Text(stringResource(R.string.common_save)) }
                    }
                }
            }
        }
    }

    if (!settings.enabled) return

    SettingsGroupLabel(stringResource(R.string.settings_linkTitles_rulesHeading))
    Hint(R.string.settings_linkTitles_rulesDescription)
    SettingsGroup {
        settings.rules.forEachIndexed { index, rule ->
            row {
                LinkRuleRow(
                    domain = rule.domain,
                    template = rule.template,
                    onEdit = { callbacks.onEditLinkRule(index) },
                    onDelete = { callbacks.onDeleteLinkRule(index) },
                )
            }
        }
        if (settings.rules.isEmpty()) row { SettingsNote(stringResource(R.string.settings_linkTitles_empty)) }
        row {
            ListItem(
                headlineContent = {
                    Text(stringResource(R.string.settings_linkTitles_add), color = MaterialTheme.colorScheme.primary)
                },
                leadingContent = {
                    Icon(Icons.Outlined.Add, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                },
                colors = groupRowColors(),
                modifier = Modifier.fillMaxWidth().clickable(onClick = callbacks.onStartNewLinkRule),
            )
        }
    }
}

@Composable
private fun currentLanguage(): String = LocalConfiguration.current.locales[0]?.language.orEmpty()

@Composable
private fun LinkRuleRow(
    domain: String,
    template: String,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    ListItem(
        headlineContent = {
            Surface(shape = RoundedCornerShape(6.dp), color = MaterialTheme.colorScheme.surfaceContainerHighest) {
                Text(
                    text = domain,
                    fontFamily = FontFamily.Monospace,
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
                )
            }
        },
        supportingContent = {
            Text(text = template, modifier = Modifier.padding(top = 6.dp))
        },
        trailingContent = {
            IconButton(onClick = onDelete) {
                Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.settings_linkTitles_remove))
            }
        },
        colors = groupRowColors(),
        modifier = Modifier.fillMaxWidth().clickable(onClick = onEdit),
    )
}

/** The editor of one domain rule, raised from the bottom like the other rule editors. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun LinkRuleEditorSheet(
    editor: LinkRuleEditor,
    callbacks: SettingsCallbacks,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(onDismissRequest = callbacks.onCancelLinkRule, sheetState = sheetState) {
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
                    text = stringResource(R.string.settings_linkTitles_rulesHeading),
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.primary,
                )
                val titleRes =
                    if (editor.index == null) R.string.native_settings_ruleNew else R.string.native_settings_ruleEdit
                Text(text = stringResource(titleRes), style = MaterialTheme.typography.headlineSmall)
                Text(
                    text = stringResource(R.string.settings_linkTitles_rulesDescription),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            OutlinedTextField(
                value = editor.domain,
                onValueChange = callbacks.onSetLinkRuleDomain,
                singleLine = true,
                label = { Text(stringResource(R.string.settings_linkTitles_domain)) },
                placeholder = { Text(stringResource(R.string.settings_linkTitles_domainPlaceholder)) },
                keyboardOptions =
                    KeyboardOptions(
                        keyboardType = KeyboardType.Uri,
                        capitalization = KeyboardCapitalization.None,
                        autoCorrectEnabled = false,
                    ),
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = editor.template,
                onValueChange = callbacks.onSetLinkRuleTemplate,
                singleLine = true,
                label = { Text(stringResource(R.string.settings_linkTitles_ruleTemplate)) },
                placeholder = {
                    Text(stringResource(R.string.settings_linkTitles_ruleTemplatePlaceholder, LINK_TITLE_PLACEHOLDER))
                },
                modifier = Modifier.fillMaxWidth(),
            )
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
            ) {
                TextButton(onClick = callbacks.onCancelLinkRule) { Text(stringResource(R.string.common_cancel)) }
                Button(onClick = callbacks.onSaveLinkRule, enabled = editor.canSave) {
                    Text(stringResource(R.string.common_save))
                }
            }
        }
    }
}
