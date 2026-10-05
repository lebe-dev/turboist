package ru.tinyops.turboist.core.network.dto

import kotlinx.serialization.Serializable

/**
 * The envelope every list endpoint answers with.
 *
 * [total] is the size of the whole result, not of [items]: it is what tells a
 * caller there is a next page without asking for one.
 */
@Serializable
data class PageDto<T>(
    val items: List<T> = emptyList(),
    val total: Int = 0,
    val limit: Int = 0,
    val offset: Int = 0,
) {
    /** True while the rows already read do not account for everything the server has. */
    val hasMore: Boolean
        get() = offset + items.size < total
}
