package ru.tinyops.turboist.core.database.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Fts4
import androidx.room.FtsOptions
import androidx.room.PrimaryKey

/**
 * The full-text index over everything the user can search by name.
 *
 * Four indexes rather than one, each declared against the table it indexes.
 * That is what buys the guarantee that matters: the index is *content-backed*,
 * so it stores no copy of the text and is maintained by triggers generated
 * alongside the schema. A row cannot be written, edited or deleted without its
 * index entry following, whether the write came from a screen, from an applied
 * change or from a cascade nobody called directly. A hand-maintained index
 * would drift the first time a write path forgot it.
 *
 * `rowid` is the indexed table's primary key, which is how a hit is turned back
 * into a row: `WHERE localId IN (SELECT rowid FROM tasks_fts WHERE ...)`.
 *
 * The tokenizer splits on Unicode character classes and folds case and
 * diacritics, because the product is used in two languages and a search for
 * "проект" must find "Проект".
 */
@Fts4(
    contentEntity = TaskRow::class,
    tokenizer = FtsOptions.TOKENIZER_UNICODE61,
)
@Entity(tableName = "tasks_fts")
data class TaskFtsRow(
    @PrimaryKey @ColumnInfo(name = "rowid") val rowId: Long,
    val title: String,
    val description: String,
)

@Fts4(
    contentEntity = ProjectRow::class,
    tokenizer = FtsOptions.TOKENIZER_UNICODE61,
)
@Entity(tableName = "projects_fts")
data class ProjectFtsRow(
    @PrimaryKey @ColumnInfo(name = "rowid") val rowId: Long,
    val title: String,
    val description: String,
)

@Fts4(
    contentEntity = LabelRow::class,
    tokenizer = FtsOptions.TOKENIZER_UNICODE61,
)
@Entity(tableName = "labels_fts")
data class LabelFtsRow(
    @PrimaryKey @ColumnInfo(name = "rowid") val rowId: Long,
    val name: String,
)

@Fts4(
    contentEntity = ContextRow::class,
    tokenizer = FtsOptions.TOKENIZER_UNICODE61,
)
@Entity(tableName = "contexts_fts")
data class ContextFtsRow(
    @PrimaryKey @ColumnInfo(name = "rowid") val rowId: Long,
    val name: String,
)
