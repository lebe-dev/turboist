package ru.tinyops.turboist.nativeapp.tasks

import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.nativeapp.quickadd.RecentProjects
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import javax.inject.Inject

/**
 * The inbox: everything written down without deciding where it belongs.
 *
 * One undivided block, because the inbox has no structure by design — it is a
 * pile to be emptied, and dividing it would suggest the pile is somewhere work
 * lives. The rows carry no project either: they are in the inbox, which is the
 * whole of what their placement says.
 */
@HiltViewModel
class InboxViewModel
    @Inject
    constructor(
        repository: TaskListRepository,
        actions: TaskListActions,
        bulk: BulkTaskActions,
        recent: RecentProjects,
        sync: SyncScheduler,
        clock: DayClock,
    ) : TaskListViewModel(clock) {
        private val sections =
            repository.observeInbox().map { tasks ->
                listOf(plainSection(key = "inbox", tasks = tasks, projectTitles = emptyMap()))
            }

        override val presenter =
            TaskListPresenter(
                viewModelScope,
                sections,
                actions,
                sync,
                bulk = bulk,
                moveOptions = repository.observeMoveOptions(),
                recentProjects = recent.observe(),
            )
    }
