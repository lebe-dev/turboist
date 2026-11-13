package ru.tinyops.turboist.core.database.entity

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * One text as the on-device language model embedded it, kept so it is never
 * worked out twice.
 *
 * Keyed by a hash of the model's name and the exact text rather than by the row
 * the text came from. A title that changes simply asks for a new key; a title
 * shared by many tasks — every run of a repeating one — is embedded once; and a
 * different model can never be mistaken for this one, because its keys differ.
 * Nothing here points into the workspace tables, so a deleted task leaves an
 * entry behind that the next tidy-up removes rather than a dangling reference.
 *
 * Derived and local-only, like the full-text indexes: never uploaded, wiped with
 * the replica on sign-out, and safe to throw away at any time — it is only the
 * cost of a minute of computation on the next start.
 *
 * @property vector the embedding as little-endian 32-bit floats, already cut to
 *   the length suggestions use.
 */
@Entity(tableName = "text_embeddings")
class TextEmbeddingRow(
    @PrimaryKey @ColumnInfo(name = "key") val key: String,
    @ColumnInfo(name = "vector", typeAffinity = ColumnInfo.BLOB) val vector: ByteArray,
)
