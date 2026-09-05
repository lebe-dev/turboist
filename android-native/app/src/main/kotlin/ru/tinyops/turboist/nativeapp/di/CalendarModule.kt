package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.network.TurboistNetwork
import ru.tinyops.turboist.core.network.api.CalendarApi
import ru.tinyops.turboist.nativeapp.calendar.CalendarCache
import ru.tinyops.turboist.nativeapp.calendar.CalendarPreference
import ru.tinyops.turboist.nativeapp.calendar.CalendarPreferences
import ru.tinyops.turboist.nativeapp.calendar.DataStoreCalendarCache
import ru.tinyops.turboist.nativeapp.tasks.TaskListRepository
import javax.inject.Singleton

/**
 * The external calendar's own small graph.
 *
 * It is assembled apart from the sync engine on purpose: nothing here touches
 * the replica, the outbox or the change history, and keeping the wiring separate
 * makes that visible at a glance rather than only in the code.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class CalendarModule {
    /**
     * One store for the process. The copy is replaced wholesale on every
     * successful read, and two instances writing the same file would race to
     * decide which answer the device wakes up with.
     */
    @Binds
    @Singleton
    abstract fun bindCalendarCache(cache: DataStoreCalendarCache): CalendarCache

    companion object {
        @Provides
        fun calendarApi(network: TurboistNetwork): CalendarApi = network.calendars

        /**
         * The calendar's two preferences, read from the replicated preference
         * document.
         *
         * The mapping lives here rather than inside the calendar layer so that
         * layer keeps no dependency on the replica at all: it is handed two
         * booleans and never learns where they came from.
         */
        @Provides
        @Singleton
        fun calendarPreferences(settings: TaskListRepository): CalendarPreferences =
            CalendarPreferences {
                settings.observeUserSettings().map {
                    CalendarPreference(enabled = it.calendarEnabled, hidePastEvents = it.calendarHidePastEvents)
                }
            }
    }
}
