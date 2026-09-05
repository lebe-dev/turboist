package ru.tinyops.turboist.nativeapp.harpoon

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.TaskDao
import ru.tinyops.turboist.core.sync.write.HarpoonTarget
import ru.tinyops.turboist.core.sync.write.SettingsWriteRepo
import javax.inject.Inject
import javax.inject.Singleton

/**
 * One end of the pair, ready to be drawn: what it is called right now.
 *
 * The name is resolved on every read rather than stored beside the entry, so a
 * task renamed anywhere is named correctly in the jump control without the pair
 * itself being touched.
 */
data class HarpoonJump(
    val entry: HarpoonEntry,
    val title: String,
)

/**
 * The jump pair, and the two gestures that change it.
 *
 * Both gestures do the same two things in one go: the device's own copy of the
 * pair is rewritten by the rule the server applies to the same gesture, and the
 * request that tells the server is queued. The control is therefore right the
 * instant it is tapped, offline included, and the account's copy catches up when
 * the queue drains.
 *
 * An entry whose row has since been deleted disappears from the pair as it is
 * read, which is what the server does with the same situation: a jump to work
 * that no longer exists is not a jump anyone wants offered.
 *
 * The two writes cannot be one transaction — the queue is in the database and
 * the pair is in a file beside it — so the queued request is written first. A
 * process killed between them leaves the account's pair changed and this
 * device's unchanged, which costs the user one tap; the other order would leave
 * a pair on screen that nothing was ever told about.
 */
@Singleton
class HarpoonRepository
    @Inject
    constructor(
        private val store: HarpoonStore,
        private val tasks: TaskDao,
        private val projects: ProjectDao,
        private val settings: SettingsWriteRepo,
    ) {
        /** The pair as it is drawn, oldest first, and again whenever anything in it changes. */
        @OptIn(ExperimentalCoroutinesApi::class)
        fun observe(): Flow<List<HarpoonJump>> =
            store.observe().flatMapLatest { entries ->
                if (entries.isEmpty()) {
                    flowOf(emptyList())
                } else {
                    combine(entries.map(::titled)) { resolved -> resolved.filterNotNull() }
                }
            }

        /**
         * Hooks a row on, evicting the older of the two if the pair is full, and
         * tells the server the same thing.
         */
        suspend fun attach(
            target: HarpoonTarget,
            localId: Long,
        ) {
            val entry = HarpoonEntry(target, localId)
            settings.attachHarpoon(target, localId)
            store.save(withHarpooned(store.observe().first(), entry))
        }

        /** Unhooks a row, and tells the server the same thing. */
        suspend fun detach(
            target: HarpoonTarget,
            localId: Long,
        ) {
            val entry = HarpoonEntry(target, localId)
            settings.detachHarpoon(target, localId)
            store.save(withoutHarpooned(store.observe().first(), entry))
        }

        /** Forgets the pair. Signing out takes it with everything else this device holds. */
        suspend fun clear() {
            store.clear()
        }

        private fun titled(entry: HarpoonEntry): Flow<HarpoonJump?> =
            when (entry.target) {
                HarpoonTarget.TASK ->
                    tasks.observeByLocalId(entry.localId).map { row ->
                        row?.let { HarpoonJump(entry, it.title) }
                    }

                HarpoonTarget.PROJECT ->
                    projects.observeByLocalId(entry.localId).map { row ->
                        row?.let { HarpoonJump(entry, it.title) }
                    }
            }
    }
