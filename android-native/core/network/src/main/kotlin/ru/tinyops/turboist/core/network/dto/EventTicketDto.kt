package ru.tinyops.turboist.core.network.dto

import kotlinx.serialization.Serializable

/**
 * Permission to open the change stream, valid once and for a few minutes.
 *
 * The stream is a plain GET whose credential travels in the query string,
 * because the client that opens it cannot set headers on the browser side and
 * the server therefore never looks for one. This client could send a header, but
 * the endpoint would ignore it, so it takes a ticket like everyone else.
 *
 * @property expiresIn seconds the ticket stays usable. Only worth reading when a
 *   handshake and the connection it authorises get separated by something slow.
 */
@Serializable
data class EventTicketDto(
    val ticket: String = "",
    val expiresIn: Int = 0,
)
