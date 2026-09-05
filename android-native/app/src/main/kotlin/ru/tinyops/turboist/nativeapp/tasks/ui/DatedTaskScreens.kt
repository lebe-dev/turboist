package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.hilt.navigation.compose.hiltViewModel
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.NextWeekViewModel
import ru.tinyops.turboist.nativeapp.tasks.TodayViewModel
import ru.tinyops.turboist.nativeapp.tasks.TomorrowViewModel
import ru.tinyops.turboist.nativeapp.tasks.WeekViewModel

/**
 * The four screens the user's calendar is read through.
 *
 * Each is the same list over a different question — what is due now, what is due
 * next, what the week holds, what has been put off — so each is a handful of
 * lines: the query lives in a view model, the rendering in [TaskListScreen], and
 * all a screen decides is what to say when it holds nothing.
 */
@Composable
fun TodayScreen(
    onOpenTask: (Task) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: TodayViewModel = hiltViewModel(),
) {
    TaskListScreen(
        viewModel = viewModel,
        empty =
            EmptyListText(
                title = stringResource(R.string.page_today_emptyTitle),
                description = stringResource(R.string.page_today_emptyDescription),
            ),
        onOpenTask = onOpenTask,
        modifier = modifier,
    )
}

@Composable
fun TomorrowScreen(
    onOpenTask: (Task) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: TomorrowViewModel = hiltViewModel(),
) {
    TaskListScreen(
        viewModel = viewModel,
        empty =
            EmptyListText(
                title = stringResource(R.string.page_tomorrow_emptyTitle),
                description = stringResource(R.string.page_tomorrow_emptyDescription),
            ),
        onOpenTask = onOpenTask,
        modifier = modifier,
    )
}

@Composable
fun WeekScreen(
    onOpenTask: (Task) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: WeekViewModel = hiltViewModel(),
) {
    TaskListScreen(
        viewModel = viewModel,
        empty =
            EmptyListText(
                title = stringResource(R.string.page_week_emptyTitle),
                description = stringResource(R.string.page_week_emptyDescription),
            ),
        onOpenTask = onOpenTask,
        modifier = modifier,
    )
}

/**
 * The planning screen has no empty state of its own: its two blocks are the two
 * ends of one decision, they name their own emptiness, and both stay on screen
 * so there is always somewhere to move work to.
 */
@Composable
fun NextWeekScreen(
    onOpenTask: (Task) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: NextWeekViewModel = hiltViewModel(),
) {
    TaskListScreen(
        viewModel = viewModel,
        empty = null,
        onOpenTask = onOpenTask,
        modifier = modifier,
    )
}
