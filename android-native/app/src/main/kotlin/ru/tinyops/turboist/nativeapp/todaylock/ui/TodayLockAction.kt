package ru.tinyops.turboist.nativeapp.todaylock.ui

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.outlined.LockOpen
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.PlainTooltip
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TooltipAnchorPosition
import androidx.compose.material3.TooltipBox
import androidx.compose.material3.TooltipDefaults
import androidx.compose.material3.rememberTooltipState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.launch
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.todaylock.TodayLockMessage
import ru.tinyops.turboist.nativeapp.todaylock.TodayLockViewModel

/** Tags the lock control in the Today screen's top bar. */
const val TODAY_LOCK_BUTTON_TAG: String = "today-lock-button"

/** Tags the button that confirms turning the lock on. */
const val TODAY_LOCK_CONFIRM_TAG: String = "today-lock-confirm"

/** The Today lock control, wired to the replica. */
@Composable
fun TodayLockControl(viewModel: TodayLockViewModel = hiltViewModel()) {
    val presenter = viewModel.presenter
    val active by presenter.active.collectAsStateWithLifecycle()
    TodayLockAction(active = active, onLock = presenter::lock, messages = presenter.messages)
}

/**
 * The Today focus lock, in the Today screen's top bar.
 *
 * Off, the open padlock offers to lock the day, and asks first: the lock cannot
 * be lifted before midnight, so it is not a thing to turn on by a stray tap. On,
 * the closed padlock only explains itself. A long press shows the explanation
 * either way, as every tooltip on the platform does; a tap on the closed one
 * shows it too, since there is nothing else a tap could mean and a phone has no
 * hover to discover it by.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TodayLockAction(
    active: Boolean,
    onLock: () -> Unit,
    modifier: Modifier = Modifier,
    messages: Flow<TodayLockMessage> = emptyFlow(),
) {
    var confirming by remember { mutableStateOf(false) }
    var announcement by remember { mutableStateOf<String?>(null) }
    val tooltip = rememberTooltipState(isPersistent = true)
    val scope = rememberCoroutineScope()
    val explanation =
        stringResource(if (active) R.string.page_today_lock_activeTooltip else R.string.page_today_lock_tooltip)
    val label = stringResource(if (active) R.string.page_today_lock_activeTooltip else R.string.page_today_lock_button)
    val locked = stringResource(R.string.page_today_lock_locked)
    val failed = stringResource(R.string.page_today_lock_failed)

    LaunchedEffect(messages, locked, failed) {
        messages.collect { message ->
            announcement =
                when (message) {
                    TodayLockMessage.LOCKED -> locked
                    TodayLockMessage.FAILED -> failed
                }
            tooltip.show()
        }
    }
    LaunchedEffect(tooltip.isVisible) { if (!tooltip.isVisible) announcement = null }

    TooltipBox(
        positionProvider = TooltipDefaults.rememberTooltipPositionProvider(TooltipAnchorPosition.Below),
        tooltip = { PlainTooltip { Text(announcement ?: explanation) } },
        state = tooltip,
        modifier = modifier,
    ) {
        IconButton(
            onClick = {
                if (active) {
                    scope.launch { tooltip.show() }
                } else {
                    confirming = true
                }
            },
            modifier = Modifier.testTag(TODAY_LOCK_BUTTON_TAG),
        ) {
            Icon(
                imageVector = if (active) Icons.Filled.Lock else Icons.Outlined.LockOpen,
                contentDescription = label,
                tint = if (active) MaterialTheme.colorScheme.primary else LocalContentColor.current,
            )
        }
    }

    if (confirming) {
        AlertDialog(
            onDismissRequest = { confirming = false },
            title = { Text(stringResource(R.string.page_today_lock_confirmTitle)) },
            text = { Text(stringResource(R.string.page_today_lock_confirmDescription)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirming = false
                        onLock()
                    },
                    modifier = Modifier.testTag(TODAY_LOCK_CONFIRM_TAG),
                ) {
                    Text(stringResource(R.string.page_today_lock_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = { confirming = false }) { Text(stringResource(R.string.common_cancel)) }
            },
        )
    }
}
