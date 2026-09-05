package ru.tinyops.turboist.core.network.api

import retrofit2.http.POST
import ru.tinyops.turboist.core.network.dto.EventTicketDto

/**
 * The handshake in front of the server's change stream.
 *
 * The stream itself is not here: it is a long-lived response that is read as it
 * arrives, which is the one thing the generated endpoint layer cannot express.
 * What it can express is the call that authorises it.
 */
interface EventsApi {
    /**
     * Mints a single-use ticket for the change stream.
     *
     * Sent without a client identifier on purpose. The server can tag a stream
     * with the client that owns it and then withhold that client's own changes,
     * which is what the web client wants — it applies its own edit optimistically
     * and an echo would make the screen fight the user. This client reacts to
     * every event the same way, by asking for changes since its stored position,
     * and applying a change it already holds writes the identical row. So there
     * is nothing to suppress, and one fewer thing that has to stay in step.
     */
    @POST("api/v1/events/ticket")
    suspend fun issueTicket(): EventTicketDto
}
