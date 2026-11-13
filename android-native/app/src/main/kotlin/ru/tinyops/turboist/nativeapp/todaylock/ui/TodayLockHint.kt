package ru.tinyops.turboist.nativeapp.todaylock.ui

import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.todaylock.TodayLockViewModel

/**
 * Whether the Today focus lock runs right now, for the places that list projects
 * and would otherwise leave the user wondering why some of them refuse a task.
 * Off unless a screen provides it, which keeps previews and tests free of the
 * replica.
 */
@Suppress("CompositionLocalAllowlist")
val LocalTodayLockActive = compositionLocalOf { false }

/** Provides [LocalTodayLockActive] from the replica to everything inside [content]. */
@Composable
fun ProvideTodayLockActive(
    viewModel: TodayLockViewModel = hiltViewModel(),
    content: @Composable () -> Unit,
) {
    val active by viewModel.presenter.active.collectAsStateWithLifecycle()
    CompositionLocalProvider(LocalTodayLockActive provides active, content = content)
}

/** The line a project picker shows while the focus lock runs; nothing otherwise. */
@Composable
fun TodayLockMoveHint(modifier: Modifier = Modifier) {
    if (!LocalTodayLockActive.current) return
    Text(
        text = stringResource(R.string.native_todayLock_moveHint),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier.padding(horizontal = 24.dp, vertical = 4.dp),
    )
}
