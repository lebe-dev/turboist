package ru.tinyops.turboist.nativeapp.harpoon.ui

import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Anchor
import androidx.compose.material.icons.outlined.Anchor
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonEntry
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonJump
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonViewModel

/**
 * The jump pair, from wherever the user is.
 *
 * It sits in the top bar rather than on a screen because that is what it is for:
 * two things the user is going back and forth between, reachable without first
 * navigating to a third place to find them.
 *
 * The same control is where a thing is hooked on and taken off again. [current]
 * is what is on screen right now, when that is a task or a project, and it is
 * the only thing the control can offer to hook on — which is exactly the surface
 * the gesture belongs to. With something on screen a tap hooks it on or takes it
 * off at once, and the icon fills while it is on the pair: that is the gesture
 * made on a task far more often than a jump, and a menu in front of it was one
 * tap too many. The jumps are then a long press away. With nothing on screen to
 * hook on, a tap opens the jumps directly.
 *
 * The whole control disappears when there is nothing to jump to and nothing on
 * screen to hook on, rather than sitting there doing nothing.
 */
@Composable
fun HarpoonAction(
    current: HarpoonEntry?,
    jumps: List<HarpoonJump>,
    onToggle: (HarpoonEntry) -> Unit,
    onOpen: (HarpoonEntry) -> Unit,
) {
    if (jumps.isEmpty() && current == null) return
    var open by remember { mutableStateOf(false) }
    val hooked = current != null && jumps.any { it.entry == current }
    val description = stringResource(if (hooked) R.string.harpoon_detach else R.string.harpoon_attach)
    val jumpsLabel = stringResource(R.string.native_harpoon_jumps)
    Box {
        Box(
            modifier =
                Modifier
                    .minimumInteractiveComponentSize()
                    .size(40.dp)
                    .clip(CircleShape)
                    .combinedClickable(
                        role = Role.Button,
                        onClick = { if (current != null) onToggle(current) else open = true },
                        onLongClickLabel = jumpsLabel.takeIf { jumps.isNotEmpty() },
                        onLongClick = { if (jumps.isNotEmpty()) open = true },
                    ).semantics { contentDescription = description },
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                imageVector = if (hooked) Icons.Filled.Anchor else Icons.Outlined.Anchor,
                contentDescription = null,
                tint = if (hooked) MaterialTheme.colorScheme.primary else LocalContentColor.current,
            )
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            for (jump in jumps) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.harpoon_jumpTo, jump.title)) },
                    onClick = {
                        open = false
                        onOpen(jump.entry)
                    },
                )
            }
        }
    }
}

/**
 * The same control, driven by a view model.
 *
 * The stateless form above is the one that is exercised in tests; this is the
 * wiring the app uses, and it holds nothing of its own so the two cannot drift.
 */
@Composable
fun HarpoonAction(
    current: HarpoonEntry?,
    onOpen: (HarpoonEntry) -> Unit,
    viewModel: HarpoonViewModel = hiltViewModel(),
) {
    val jumps by viewModel.jumps.collectAsStateWithLifecycle()
    HarpoonAction(
        current = current,
        jumps = jumps,
        onToggle = viewModel::toggle,
        onOpen = onOpen,
    )
}
