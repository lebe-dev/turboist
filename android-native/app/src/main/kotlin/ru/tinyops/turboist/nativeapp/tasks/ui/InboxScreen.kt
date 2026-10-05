package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.hilt.navigation.compose.hiltViewModel
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.InboxViewModel

/**
 * The inbox, as a list.
 *
 * The same list every other screen uses, over the one query that asks what has
 * been written down without a home. When it is empty it says what the inbox is
 * for rather than that it is empty — an empty inbox is the goal, not a problem
 * to fix.
 */
@Composable
fun InboxScreen(
    onOpenTask: (Task) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: InboxViewModel = hiltViewModel(),
) {
    TaskListScreen(
        viewModel = viewModel,
        empty =
            EmptyListText(
                title = stringResource(R.string.page_inbox_emptyTitle),
                description = stringResource(R.string.page_inbox_subtitle),
            ),
        onOpenTask = onOpenTask,
        modifier = modifier,
    )
}
