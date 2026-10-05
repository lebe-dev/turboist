package ru.tinyops.turboist.core.network

import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

/**
 * The API answers every failure with the same envelope, and the point of the
 * mapping is that a caller never has to read a status code to know what happened.
 * These tests pin the translation from one to the other.
 */
class ErrorMappingTest {
    @Test
    fun `a blocked task carries the ids of what is blocking it`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                errorResponse(
                    409,
                    ApiErrorCodes.TASK_BLOCKED,
                    "task is blocked",
                    details = """{"blockerIds":[7,11]}""",
                ),
            )

            val failure =
                assertFailsWith<ApiException.Business> {
                    runBlocking { fixture.network.tasks.complete(3, completeBody()) }
                }

            assertEquals(ApiErrorCodes.TASK_BLOCKED, failure.code)
            assertEquals(409, failure.status)
            assertEquals(listOf(7L, 11L), failure.blockerIds)
        }
    }

    @Test
    fun `a replaced history tells the replica which epoch to start over on`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                errorResponse(
                    409,
                    ApiErrorCodes.SYNC_EPOCH_MISMATCH,
                    "sync epoch mismatch",
                    details = """{"epoch":4}""",
                ),
            )

            val failure =
                assertFailsWith<ApiException.SyncEpochMismatch> {
                    runBlocking { fixture.network.sync.changes(since = 10, epoch = 3) }
                }

            assertEquals(4L, failure.currentEpoch)
        }
    }

    @Test
    fun `a pruned cursor reports the gap it cannot bridge`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                errorResponse(
                    410,
                    ApiErrorCodes.SYNC_CURSOR_EXPIRED,
                    "sync cursor expired",
                    details = """{"epoch":2,"oldestRetained":900}""",
                ),
            )

            val failure =
                assertFailsWith<ApiException.SyncCursorExpired> {
                    runBlocking { fixture.network.sync.changes(since = 10) }
                }

            assertEquals(2L, failure.epoch)
            assertEquals(900L, failure.oldestRetained)
        }
    }

    @Test
    fun `a rejected session is an auth failure, and an aged-out one says so`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(errorResponse(401, ApiErrorCodes.AUTH_INVALID, "invalid credentials"))
            val invalid =
                assertFailsWith<ApiException.Auth> {
                    runBlocking { fixture.network.tasks.get(1) }
                }
            assertEquals(false, invalid.isExpiredAccess)

            fixture.server.enqueue(errorResponse(401, ApiErrorCodes.AUTH_EXPIRED, "access token expired"))
            val expired =
                assertFailsWith<ApiException.Auth> {
                    runBlocking { fixture.network.tasks.get(1) }
                }
            assertTrue(expired.isExpiredAccess)
        }
    }

    @Test
    fun `an instance without an account is a setup demand, not a server failure`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(errorResponse(503, ApiErrorCodes.SETUP_REQUIRED, "setup required"))

            assertFailsWith<ApiException.SetupRequired> {
                runBlocking { fixture.network.tasks.get(1) }
            }
        }
    }

    @Test
    fun `too many attempts is its own answer`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(errorResponse(429, ApiErrorCodes.AUTH_RATE_LIMITED, "too many requests"))

            assertFailsWith<ApiException.RateLimited> {
                runBlocking { fixture.network.auth.login(loginBody()) }
            }
        }
    }

    @Test
    fun `a broken server is worth retrying, so it maps to its own family`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(errorResponse(500, ApiErrorCodes.INTERNAL_ERROR, "boom"))

            val failure =
                assertFailsWith<ApiException.Server> {
                    runBlocking { fixture.network.tasks.get(1) }
                }
            assertEquals(500, failure.status)
        }
    }

    @Test
    fun `a proxy answering with something that is not an envelope still maps by status`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                MockResponse.Builder().code(
                    502,
                ).setHeader("Content-Type", "text/html").body("<html>bad gateway</html>").build(),
            )

            val failure =
                assertFailsWith<ApiException.Server> {
                    runBlocking { fixture.network.tasks.get(1) }
                }
            assertEquals(ApiErrorCodes.UNKNOWN_ERROR, failure.code)
            assertEquals(502, failure.status)
        }
    }

    @Test
    fun `a server that cannot be reached is a network failure, never a business one`() {
        val fixture = NetworkFixture()
        fixture.close()

        val failure =
            assertFailsWith<ApiException.Network> {
                runBlocking { fixture.network.tasks.get(1) }
            }
        assertEquals(0, failure.status)
        assertEquals(ApiErrorCodes.NETWORK_ERROR, failure.code)
    }

    @Test
    fun `a request made before an address is configured never leaves the device`() {
        val network = TurboistNetwork.create(ServerUrl())

        assertFailsWith<ApiException.Network> {
            runBlocking { network.tasks.get(1) }
        }
    }
}
