package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Query
import androidx.room.Upsert
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.AppSettingsRow
import ru.tinyops.turboist.core.database.entity.UserSettingsRow
import ru.tinyops.turboist.core.database.entity.UserStateRow

/**
 * The three single-row documents.
 *
 * Each read answers `null` until the document has been synced for the first
 * time. That is a real state — a replica that has never reached a server holds
 * no preferences — and it is deliberately not papered over with an invented
 * default here: what the product means by "unset" belongs to the layer that
 * decodes the document, not to the table it sits in.
 *
 * The queries take no key. Each table holds exactly one row by construction, so
 * naming the row would only create a way to ask for the wrong one.
 */
@Dao
interface SettingsDao {
    @Upsert
    suspend fun saveUserSettings(row: UserSettingsRow)

    @Query("SELECT * FROM user_settings LIMIT 1")
    suspend fun userSettings(): UserSettingsRow?

    @Query("SELECT * FROM user_settings LIMIT 1")
    fun observeUserSettings(): Flow<UserSettingsRow?>

    @Upsert
    suspend fun saveAppSettings(row: AppSettingsRow)

    @Query("SELECT * FROM app_settings LIMIT 1")
    suspend fun appSettings(): AppSettingsRow?

    @Query("SELECT * FROM app_settings LIMIT 1")
    fun observeAppSettings(): Flow<AppSettingsRow?>

    @Upsert
    suspend fun saveUserState(row: UserStateRow)

    @Query("SELECT * FROM user_state LIMIT 1")
    suspend fun userState(): UserStateRow?

    @Query("SELECT * FROM user_state LIMIT 1")
    fun observeUserState(): Flow<UserStateRow?>
}
