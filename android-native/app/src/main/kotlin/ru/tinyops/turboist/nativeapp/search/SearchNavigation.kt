package ru.tinyops.turboist.nativeapp.search

import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task

/**
 * Where a result leads.
 *
 * The screen finds things; the graph decides what opening one means and how it
 * is addressed. Passing whole records rather than ids is what makes that
 * division work: a record created on this device has no server-side address yet,
 * and only the graph knows that a destination is reached by one.
 */
data class SearchNavigation(
    val onOpenTask: (Task) -> Unit,
    val onOpenProject: (Project) -> Unit,
    val onOpenLabel: (Label) -> Unit,
    val onOpenContext: (Context) -> Unit,
)
