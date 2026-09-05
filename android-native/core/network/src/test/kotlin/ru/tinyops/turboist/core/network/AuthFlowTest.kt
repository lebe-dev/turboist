package ru.tinyops.turboist.core.network

import kotlinx.coroutines.runBlocking
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.RecordedRequest
import ru.tinyops.turboist.core.network.auth.AccessTokenRefresher
import ru.tinyops.turboist.core.network.auth.AccessTokenSource
import ru.tinyops.turboist.core.network.dto.LoginOutcome
import ru.tinyops.turboist.core.network.dto.LoginRequest
import ru.tinyops.turboist.core.network.dto.OtpLoginRequest
import ru.tinyops.turboist.core.network.dto.RefreshRequest
import ru.tinyops.turboist.core.network.dto.SetupRequest
import ru.tinyops.turboist.core.network.dto.toOutcome
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference
import kotlin.concurrent.thread
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The session lifetime, driven end to end against a mock server.
 *
 * The interesting property is not that a refresh happens — it is that it happens
 * exactly once and is invisible to the caller: an access token expiring mid-use is
 * the normal course of a 15-minute token, not an error the app should surface.
 */
class AuthFlowTest {
    @Test
    fun `sign in, use the session, survive the token expiring`() {
        val accessToken = AtomicReference<String?>(null)
        val refreshToken = AtomicReference("refresh-1")
        val refreshCalls = AtomicInteger()

        val fixture =
            NetworkFixture(
                tokens = { accessToken.get() },
                refresher = { expired ->
                    refreshCalls.incrementAndGet()
                    // The refresher owns the rotating token and the storage behind
                    // it; the HTTP layer only ever learns the new access token.
                    assertEquals("access-1", expired)
                    val rotated = "access-2"
                    accessToken.set(rotated)
                    refreshToken.set("refresh-2")
                    rotated
                },
            )

        fixture.use {
            fixture.server.enqueue(jsonResponse(200, sessionJson()))
            fixture.server.enqueue(jsonResponse(200, """{"id":42,"title":"write it down","status":"open"}"""))
            fixture.server.enqueue(errorResponse(401, ApiErrorCodes.AUTH_EXPIRED, "access token expired"))
            fixture.server.enqueue(jsonResponse(200, """{"id":42,"title":"write it down","status":"open"}"""))

            val outcome = runBlocking { fixture.network.auth.login(loginBody()) }.toOutcome()
            val session = assertIs<LoginOutcome.Session>(outcome).session
            assertEquals("access-1", session.access)
            accessToken.set(session.access)

            val first = runBlocking { fixture.network.tasks.get(42) }
            assertEquals(42L, first.id)

            val second = runBlocking { fixture.network.tasks.get(42) }
            assertEquals(42L, second.id)

            assertEquals(1, refreshCalls.get())
            assertEquals("access-2", accessToken.get())

            // The login carried no session; the first read carried the original
            // token; the rejected read carried it too; the retry carried the new one.
            assertNull(fixture.server.takeRequest().headers[ApiHeaders.AUTHORIZATION])
            assertEquals("Bearer access-1", fixture.server.takeRequest().headers[ApiHeaders.AUTHORIZATION])
            assertEquals("Bearer access-1", fixture.server.takeRequest().headers[ApiHeaders.AUTHORIZATION])
            assertEquals("Bearer access-2", fixture.server.takeRequest().headers[ApiHeaders.AUTHORIZATION])
        }
    }

    @Test
    fun `several requests rejected at once produce one refresh, not one each`() {
        val accessToken = AtomicReference<String?>("stale")
        val refreshCalls = AtomicInteger()
        val released = CountDownLatch(1)

        val fixture =
            NetworkFixture(
                tokens = { accessToken.get() },
                refresher = {
                    refreshCalls.incrementAndGet()
                    // Hold the lock long enough for every other caller to pile up
                    // behind it, which is the race this test exists for.
                    released.await(5, TimeUnit.SECONDS)
                    accessToken.set("fresh")
                    "fresh"
                },
            )

        fixture.use {
            val callers = 4
            // Answering by what the request carries rather than from a queue: the
            // four callers race, so which request arrives first is undefined, and a
            // queued script would hand one caller another's answer.
            fixture.server.dispatcher =
                object : Dispatcher() {
                    override fun dispatch(request: RecordedRequest): MockResponse =
                        if (request.headers[ApiHeaders.AUTHORIZATION] == "Bearer fresh") {
                            jsonResponse(200, """{"id":42,"title":"write it down","status":"open"}""")
                        } else {
                            errorResponse(401, ApiErrorCodes.AUTH_EXPIRED, "access token expired")
                        }
                }

            val started = CountDownLatch(callers)
            val failures = AtomicReference<Throwable?>(null)
            val threads =
                (1..callers).map {
                    thread {
                        started.countDown()
                        try {
                            runBlocking { fixture.network.tasks.get(42) }
                        } catch (e: Throwable) {
                            failures.compareAndSet(null, e)
                        }
                    }
                }
            assertTrue(started.await(5, TimeUnit.SECONDS))
            released.countDown()
            threads.forEach { it.join(10_000) }

            assertNull(failures.get())
            assertEquals(1, refreshCalls.get())
        }
    }

    @Test
    fun `a refusal that is not an expiry is passed through untouched`() {
        val refreshCalls = AtomicInteger()
        val fixture =
            NetworkFixture(
                tokens = AccessTokenSource { "whatever" },
                refresher =
                    AccessTokenRefresher {
                        refreshCalls.incrementAndGet()
                        "never-used"
                    },
            )

        fixture.use {
            fixture.server.enqueue(errorResponse(401, ApiErrorCodes.AUTH_INVALID, "invalid credentials"))

            val failure =
                runCatching { runBlocking { fixture.network.tasks.get(1) } }.exceptionOrNull()

            assertIs<ApiException.Auth>(failure)
            assertEquals(0, refreshCalls.get())
        }
    }

    @Test
    fun `a request that carried no token is repaired rather than left refused`() {
        // The shape every authenticated call takes after the app launched with no
        // network: the session is stored, nothing has been minted from it yet, and
        // the server refuses for want of credentials rather than because of them.
        // Letting that refusal stand would strand the app on stale data for the
        // rest of the process.
        val accessToken = AtomicReference<String?>(null)
        val refreshCalls = AtomicInteger()

        val fixture =
            NetworkFixture(
                tokens = { accessToken.get() },
                refresher = { expired ->
                    refreshCalls.incrementAndGet()
                    assertNull(expired, "there was no token to expire")
                    accessToken.set("access-1")
                    "access-1"
                },
            )

        fixture.use {
            fixture.server.enqueue(errorResponse(401, ApiErrorCodes.AUTH_INVALID, "missing authorization header"))
            fixture.server.enqueue(jsonResponse(200, """{"id":42,"title":"write it down","status":"open"}"""))

            val task = runBlocking { fixture.network.tasks.get(42) }

            assertEquals(42L, task.id)
            assertEquals(1, refreshCalls.get())
            assertNull(fixture.server.takeRequest().headers[ApiHeaders.AUTHORIZATION])
            assertEquals("Bearer access-1", fixture.server.takeRequest().headers[ApiHeaders.AUTHORIZATION])
        }
    }

    @Test
    fun `a second factor is a challenge to finish, not a failed login`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, """{"otpRequired":true,"ticket":"ticket-1"}"""))
            fixture.server.enqueue(jsonResponse(200, sessionJson(totpEnabled = true)))

            val outcome = runBlocking { fixture.network.auth.login(loginBody()) }.toOutcome()
            val challenge = assertIs<LoginOutcome.OtpRequired>(outcome)
            assertEquals("ticket-1", challenge.ticket)

            val session = runBlocking { fixture.network.auth.loginWithOtp(OtpLoginRequest(challenge.ticket, "123456")) }
            assertEquals("access-1", session.access)
            assertTrue(session.user.totpEnabled)
        }
    }

    @Test
    fun `the sign-in endpoints never trigger a refresh of their own`() {
        val refreshCalls = AtomicInteger()
        val fixture =
            NetworkFixture(
                tokens = AccessTokenSource { "stale" },
                refresher =
                    AccessTokenRefresher {
                        refreshCalls.incrementAndGet()
                        "fresh"
                    },
            )

        fixture.use {
            fixture.server.enqueue(errorResponse(401, ApiErrorCodes.AUTH_EXPIRED, "access token expired"))

            runCatching { runBlocking { fixture.network.auth.login(loginBody()) } }

            assertEquals(0, refreshCalls.get())
            assertNull(fixture.server.takeRequest().headers[ApiHeaders.AUTHORIZATION])
        }
    }
}

/**
 * What the sign-in bodies actually put on the wire.
 *
 * The layer's JSON deliberately omits properties still holding their default, so
 * that a PATCH body carries only what the user changed. `clientKind` is the one
 * place where that rule would silently do the wrong thing: it is constant for
 * this client, so it looks exactly like an untouched default and would be
 * dropped — and the server rejects a sign-in that does not name a client kind.
 * These tests pin the encoded body rather than the object, because the object
 * was always right; only what left the device was wrong.
 */
class ClientKindOnTheWireTest {
    @Test
    fun `account creation names the client kind`() {
        val body = TurboistJson.encodeToString(SetupRequest(username = "u", password = "p", clientKind = "android"))
        assertTrue(""""clientKind":"android"""" in body, "setup body must name the client kind, was: $body")
    }

    @Test
    fun `signing in names the client kind`() {
        val body = TurboistJson.encodeToString(LoginRequest(username = "u", password = "p", clientKind = "android"))
        assertTrue(""""clientKind":"android"""" in body, "login body must name the client kind, was: $body")
    }

    /**
     * The property still has a default, so a caller cannot forget it; this pins
     * that leaving it alone encodes the same thing as passing it.
     */
    @Test
    fun `the client kind is written even when left at its default`() {
        assertTrue(""""clientKind":"android"""" in TurboistJson.encodeToString(SetupRequest("u", "p")))
        assertTrue(""""clientKind":"android"""" in TurboistJson.encodeToString(LoginRequest("u", "p")))
    }

    /** The omit-defaults rule that makes PATCH bodies work is untouched elsewhere. */
    @Test
    fun `other defaults are still omitted`() {
        assertEquals("""{"refresh":"r"}""", TurboistJson.encodeToString(RefreshRequest(refresh = "r")))
    }
}
