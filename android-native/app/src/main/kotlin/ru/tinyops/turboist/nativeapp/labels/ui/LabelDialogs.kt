package ru.tinyops.turboist.nativeapp.labels.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.projects.ui.NAMED_COLOR_CHOICES
import ru.tinyops.turboist.nativeapp.projects.ui.projectTint

/** What a label editor was filled in with when it was confirmed. */
data class LabelDraft(
    val name: String,
    val color: String,
)

/**
 * Names and colours a label.
 *
 * A sheet rather than a dialog, and the same one the capture surface uses: it is
 * a form to be typed into, and on a phone a sheet keeps the field above the
 * keyboard where a centred dialog would sit behind it.
 *
 * One surface for both making a label and changing one: the questions are the
 * same two, and two forms that asked them separately would answer them
 * differently the first time either was touched.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LabelEditorSheet(
    initial: LabelDraft?,
    onConfirm: (LabelDraft) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheetState, modifier = modifier) {
        LabelEditorContent(
            initial = initial,
            onConfirm = {
                onConfirm(it)
                onDismiss()
            },
            onDismiss = onDismiss,
        )
    }
}

/**
 * What the editor holds: what the label is called, and what colour it is drawn
 * in.
 *
 * The name is the only required part — a colour is decoration, and a label with
 * none is drawn in the theme's own ink — so the confirm button stays out of
 * reach until there is a name to save.
 */
@Composable
fun LabelEditorContent(
    initial: LabelDraft?,
    onConfirm: (LabelDraft) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var name by remember { mutableStateOf(initial?.name.orEmpty()) }
    var color by remember { mutableStateOf(initial?.color.orEmpty()) }
    val nameLabel = stringResource(R.string.common_name)

    Column(
        verticalArrangement = Arrangement.spacedBy(12.dp),
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        Text(
            text =
                stringResource(
                    if (initial == null) R.string.dialog_label_newTitle else R.string.dialog_label_editTitle,
                ),
            style = MaterialTheme.typography.titleMedium,
        )
        Text(
            text = stringResource(R.string.dialog_label_description),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        OutlinedTextField(
            value = name,
            onValueChange = { name = it },
            label = { Text(nameLabel) },
            placeholder = { Text(stringResource(R.string.dialog_label_namePlaceholder)) },
            singleLine = true,
            // The label floats above the field once there is text in it, so the
            // field is named again here: a reader that only ever hears the field
            // would otherwise be asked to fill in an unnamed box.
            modifier = Modifier.fillMaxWidth().semantics { contentDescription = nameLabel },
        )
        Text(
            text = stringResource(R.string.common_color),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        ColorChoices(chosen = color, onChoose = { color = it })
        Row(
            horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
            modifier = Modifier.fillMaxWidth(),
        ) {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
            TextButton(
                enabled = name.isNotBlank(),
                onClick = { onConfirm(LabelDraft(name.trim(), color)) },
            ) {
                Text(stringResource(if (initial == null) R.string.common_create else R.string.common_save))
            }
        }
    }
}

/**
 * The palette, as a row of swatches.
 *
 * The chosen one is ringed rather than ticked: a tick drawn over a colour is
 * unreadable on half the palette, and the ring stays visible whatever is under
 * it. Every swatch names its colour to an assistive reader, which is the only
 * way the choice is available to somebody who cannot see it.
 */
@Composable
private fun ColorChoices(
    chosen: String,
    onChoose: (String) -> Unit,
) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        for (name in NAMED_COLOR_CHOICES) {
            val tint = projectTint(name) ?: continue
            Swatch(
                tint = tint,
                name = name,
                selected = name.equals(chosen, ignoreCase = true),
                onClick = { onChoose(name) },
            )
        }
    }
}

@Composable
private fun Swatch(
    tint: Color,
    name: String,
    selected: Boolean,
    onClick: () -> Unit,
) {
    val ring = if (selected) MaterialTheme.colorScheme.onSurface else Color.Transparent
    Row(
        modifier =
            Modifier
                .size(SWATCH_SIZE)
                .clip(MaterialTheme.shapes.small)
                .background(tint)
                .border(SWATCH_RING, ring, MaterialTheme.shapes.small)
                .clickable(onClick = onClick)
                .semantics { contentDescription = name },
    ) {}
}

private val SWATCH_SIZE = 24.dp
private val SWATCH_RING = 2.dp

/**
 * Asks before a label is taken away.
 *
 * The number of tasks about to lose it is part of the question, not a detail:
 * deleting a label is hard on both sides and cannot be undone, and "this is on
 * forty tasks" is the one fact that changes the answer. The work itself is
 * untouched — a tagging is an edge, not the task.
 */
@Composable
fun ConfirmLabelDeleteDialog(
    taggedTasks: Int,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.page_label_confirmDeleteTitle)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.page_label_confirmDeleteDesc))
                if (taggedTasks > 0) {
                    Text(
                        // The shared wording carries no type for its values, so the
                        // count is handed over as text.
                        text = stringResource(R.string.page_labels_row_total, taggedTasks.toString()),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = {
                    onConfirm()
                    onDismiss()
                },
            ) { Text(stringResource(R.string.common_delete)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}
