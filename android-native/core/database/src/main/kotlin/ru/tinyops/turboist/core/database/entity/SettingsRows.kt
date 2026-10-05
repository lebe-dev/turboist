package ru.tinyops.turboist.core.database.entity

import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * The id every single-row table carries. There is one user, one installation and
 * one interface state, so these tables hold exactly one row and a fixed key is
 * all that is needed to address it.
 */
const val SINGLE_ROW_ID: Int = 1

/**
 * The user's own preferences, the installation's rules, and the interface state
 * that follows the user between devices.
 *
 * All three are stored as the payload the server sent, verbatim, rather than
 * spread across typed columns. The server keeps each of them as one document —
 * not as a table — and echoes writes back through the same document, key by key
 * for the interface state. Spreading a document into columns would silently drop
 * every key this build has not been taught about, and a write would then hand
 * the server back a document with those keys missing. Storing the document whole
 * makes an unknown key a thing this build ignores rather than a thing it
 * destroys.
 *
 * Decoding the payload into the domain types is the mapping layer's job, the
 * same layer that decodes every other payload; the replica's contract here is
 * "the latest document, and when it arrived".
 */
@Entity(tableName = "user_settings")
data class UserSettingsRow(
    @PrimaryKey val id: Int = SINGLE_ROW_ID,
    val payload: String,
    val updatedAt: Long,
)

@Entity(tableName = "app_settings")
data class AppSettingsRow(
    @PrimaryKey val id: Int = SINGLE_ROW_ID,
    val payload: String,
    val updatedAt: Long,
)

@Entity(tableName = "user_state")
data class UserStateRow(
    @PrimaryKey val id: Int = SINGLE_ROW_ID,
    val payload: String,
    val updatedAt: Long,
)
