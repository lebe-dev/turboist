package ru.tinyops.turboist.core.database.search

import androidx.room.withTransaction
import ru.tinyops.turboist.core.database.TurboistDatabase

/** The four indexes, each named after the table it is derived from. */
private val FULL_TEXT_INDEXES = listOf("tasks_fts", "projects_fts", "labels_fts", "contexts_fts")

/**
 * Rebuilds every full-text index from the tables it describes.
 *
 * The indexes hold no text of their own — each one reads its content out of the
 * table it indexes and is kept current by triggers — so they are entirely
 * derived, and throwing one away and building it again always produces exactly
 * what it should have held. That is what makes this safe to run at any moment
 * and what makes it the repair for the one thing triggers cannot cover: a table
 * emptied or filled wholesale, where the rows changed without a statement per
 * row for a trigger to ride along on.
 *
 * It runs after a complete reseed for that reason, not as routine upkeep. Every
 * ordinary write — a screen's, an applied change, a cascade — maintains the
 * index by itself, and rebuilding after each of those would be reading the whole
 * workspace to learn something already known.
 *
 * All four are rebuilt in one transaction: search reads across the four kinds as
 * one answer, and a half-rebuilt set would report that half the workspace had
 * vanished.
 */
suspend fun TurboistDatabase.rebuildSearchIndex() {
    withTransaction {
        for (index in FULL_TEXT_INDEXES) {
            openHelper.writableDatabase.execSQL("INSERT INTO $index($index) VALUES('rebuild')")
        }
    }
}
