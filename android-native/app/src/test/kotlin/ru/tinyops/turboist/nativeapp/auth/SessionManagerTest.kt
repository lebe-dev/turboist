package ru.tinyops.turboist.nativeapp.auth

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withTimeout
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.ServerUrl
import ru.tinyops.turboist.core.network.dto.LoginOutcome
import ru.tinyops.turboist.core.network.dto.SessionDto
import ru.tinyops.turboist.nativeapp.session.SessionState
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlin.concurrent.thread
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

private const val ADDRESS = "https://todo.example.com/"

/**
 * The session, end to end, with the server and the storage replaced by
 * stand-ins.
 *
 * What is being tested is not that the calls happen but what surrounds them:
 * which failure keeps a token and which throws it away, that a rotation is
 * stored before it is used, and that signing out never quietly discards work.
 */
class SessionManagerTest {
    private fun manager(
        gateway: FakeAuthGateway = FakeAuthGateway(),
        tokens: FakeRefreshTokenStore = FakeRefreshTokenStore(),
        addresses: FakeServerAddressStore = FakeServerAddressStore(),
        replica: FakeLocalReplica = FakeLocalReplica(),
        serverUrl: ServerUrl = ServerUrl(),
        networks: FakeNetworkAvailability = FakeNetworkAvailability(),
        accessTokens: SessionTokens = SessionTokens(),
        cleartext: CleartextPolicy = CleartextPolicy(allowed = false),
    ) = SessionManager(
        gateway = gateway,
        accessTokens = accessTokens,
        refreshTokens = tokens,
        serverAddresses = addresses,
        serverUrl = serverUrl,
        replica = replica,
        networks = networks,
        cleartext = cleartext,
        scope = CoroutineScope(Dispatchers.Unconfined),
    )

    // --- launch -----------------------------------------------------------

    @Test
    fun `a fresh install opens on the server screen`() =
        runTest {
            val session = manager()

            session.boot()

            assertEquals(SessionState.LoggedOut, session.state.value)
            assertEquals(AuthStep.Connect, session.authStep.value)
        }

    @Test
    fun `a known server with no account opens on the setup screen`() =
        runTest {
            val gateway = FakeAuthGateway().apply { setup = SetupProbe.REQUIRED }
            val session = manager(gateway = gateway, addresses = FakeServerAddressStore(ADDRESS))

            session.boot()

            assertEquals(SessionState.LoggedOut, session.state.value)
            assertEquals(AuthStep.Setup, session.authStep.value)
        }

    @Test
    fun `a stored session that renews opens the app`() =
        runTest {
            val gateway = FakeAuthGateway()
            gateway.rotations += Result.success(sessionOf("access-2", "refresh-2"))
            val tokens = FakeRefreshTokenStore("refresh-1")
            val session = manager(gateway = gateway, tokens = tokens, addresses = FakeServerAddressStore(ADDRESS))

            session.boot()

            assertEquals(SessionState.LoggedIn, session.state.value)
            assertFalse(session.unverifiedSession.value)
            assertEquals(listOf("refresh-1"), gateway.presentedRefreshTokens)
            assertEquals(
                listOf("refresh-2"),
                tokens.writes,
                "the rotated token is stored before anything uses the session it belongs to",
            )
            assertEquals("refresh-2", tokens.stored(), "the rotated token replaces the one just spent")
        }

    @Test
    fun `an unreachable server opens the app rather than signing the user out`() =
        runTest {
            // The whole point of a replica: no signal is not a refusal, and the
            // stored token is almost certainly still good.
            val gateway = FakeAuthGateway()
            val tokens = FakeRefreshTokenStore("refresh-1")
            val replica = FakeLocalReplica(unsent = 2)
            val session =
                manager(
                    gateway = gateway,
                    tokens = tokens,
                    addresses = FakeServerAddressStore(ADDRESS),
                    replica = replica,
                )

            session.boot()

            assertEquals(SessionState.LoggedIn, session.state.value)
            assertTrue(session.unverifiedSession.value, "the app has to say it could not check the session")
            assertEquals("refresh-1", tokens.stored(), "an unreachable server must not cost the user their token")
            assertEquals(0, replica.wipes)
        }

    @Test
    fun `a refused token ends the session but keeps the replica and the queue`() =
        runTest {
            val gateway = FakeAuthGateway()
            gateway.rotations += Result.failure(rejection())
            val tokens = FakeRefreshTokenStore("refresh-1")
            val replica = FakeLocalReplica(unsent = 3)
            val session =
                manager(
                    gateway = gateway,
                    tokens = tokens,
                    addresses = FakeServerAddressStore(ADDRESS),
                    replica = replica,
                )

            session.boot()

            assertEquals(SessionState.LoggedOut, session.state.value)
            assertEquals(AuthStep.SignIn, session.authStep.value)
            assertNull(tokens.stored(), "a token the server refused must not be presented again")
            // Signing back in has to resume the unsent work, not start from an
            // empty device — so nothing local is thrown away here.
            assertEquals(0, replica.wipes)
            assertEquals(3, replica.unsent)
        }

    @Test
    fun `an unusable stored address is forgotten instead of crashing every launch`() =
        runTest {
            val addresses = FakeServerAddressStore("not an address")
            val session = manager(addresses = addresses)

            session.boot()

            assertEquals(AuthStep.Connect, session.authStep.value)
            assertNull(addresses.stored())
        }

    // --- leaving offline mode ---------------------------------------------

    @Test
    fun `a session the launch could not prove is checked again when the network returns`() =
        runBlocking {
            // Opening on the replica is only half the rule. The other half is that
            // it ends by itself: a session left unproven with no way back would
            // keep the app on stale data until the user thought to kill it.
            //
            // Real dispatchers rather than virtual time, because what is under
            // test is that a platform callback on some other thread reaches the
            // session at all.
            val gateway = FakeAuthGateway()
            val tokens = FakeRefreshTokenStore("refresh-1")
            val networks = FakeNetworkAvailability()
            val accessTokens = SessionTokens()
            val session =
                manager(
                    gateway = gateway,
                    tokens = tokens,
                    addresses = FakeServerAddressStore(ADDRESS),
                    networks = networks,
                    accessTokens = accessTokens,
                )

            session.start()
            assertTrue(session.unverifiedSession.value, "the launch could not reach the server")
            assertNull(accessTokens.accessToken(), "an unproven session has no access token to show for it")

            gateway.rotations += Result.success(sessionOf("access-2", "refresh-2"))
            networks.announce()

            withTimeout(5_000) { session.unverifiedSession.first { !it } }
            assertEquals(SessionState.LoggedIn, session.state.value)
            assertEquals("access-2", accessTokens.accessToken())
            assertEquals("refresh-2", tokens.stored())
        }

    @Test
    fun `a network that still cannot reach the server leaves the app on the replica`() =
        runTest {
            // Joining a network is not reaching the server: a captive portal, or a
            // server that is simply down, must not cost the user their session.
            val gateway = FakeAuthGateway()
            val tokens = FakeRefreshTokenStore("refresh-1")
            val session =
                manager(gateway = gateway, tokens = tokens, addresses = FakeServerAddressStore(ADDRESS))

            session.boot()
            session.revalidateSession()

            assertTrue(session.unverifiedSession.value)
            assertEquals(SessionState.LoggedIn, session.state.value)
            assertEquals("refresh-1", tokens.stored())
            assertEquals(2, gateway.renewals, "each network event is worth one attempt, and no more")
        }

    @Test
    fun `a network event does not rotate a session that is already proved`() =
        runTest {
            // Rotation spends a single-use token. Doing it because the wifi changed
            // would be a round trip and a risk for no gain.
            val gateway = FakeAuthGateway()
            gateway.rotations += Result.success(sessionOf("access-2", "refresh-2"))
            val session =
                manager(
                    gateway = gateway,
                    tokens = FakeRefreshTokenStore("refresh-1"),
                    addresses = FakeServerAddressStore(ADDRESS),
                )

            session.boot()
            assertFalse(session.unverifiedSession.value)

            session.revalidateSession()

            assertEquals(1, gateway.renewals)
        }

    @Test
    fun `a request that carried no token at all repairs the session instead of failing`() =
        runTest {
            // What every authenticated call looks like after a launch with no
            // signal: there is a stored session and nothing minted from it yet, so
            // the server sees no credentials rather than bad ones.
            val gateway = FakeAuthGateway()
            val tokens = FakeRefreshTokenStore("refresh-1")
            val session =
                manager(gateway = gateway, tokens = tokens, addresses = FakeServerAddressStore(ADDRESS))
            session.boot()
            assertTrue(session.unverifiedSession.value)

            gateway.rotations += Result.success(sessionOf("access-2", "refresh-2"))
            val replacement = session.renewAccessToken(null)

            assertEquals("access-2", replacement)
            assertFalse(session.unverifiedSession.value)
            assertEquals("refresh-2", tokens.stored())
        }

    @Test
    fun `a device with no stored session asks nothing when the network returns`() =
        runTest {
            val gateway = FakeAuthGateway()
            val session = manager(gateway = gateway, addresses = FakeServerAddressStore(ADDRESS))

            session.boot()
            session.revalidateSession()

            assertEquals(SessionState.LoggedOut, session.state.value)
            assertEquals(AuthStep.SignIn, session.authStep.value)
            assertEquals(0, gateway.renewals, "there is no token to present, so there is nothing to ask")
        }

    // --- rotation ---------------------------------------------------------

    @Test
    fun `a caller that arrives after a rotation takes the new token instead of rotating again`() =
        runTest {
            // Spending the refresh token twice is what the server reads as theft,
            // and it answers by killing the session. So a second caller holding a
            // token that has already been replaced must not ask for another.
            val gateway = FakeAuthGateway()
            gateway.rotations += Result.success(sessionOf("access-2", "refresh-2"))
            val session =
                manager(
                    gateway = gateway,
                    tokens = FakeRefreshTokenStore("refresh-1"),
                    addresses = FakeServerAddressStore(ADDRESS),
                )
            session.boot()

            val replacement = session.renewAccessToken("access-1")

            assertEquals("access-2", replacement)
            assertEquals(1, gateway.renewals)
        }

    @Test
    fun `two requests rejected at once produce one rotation`() {
        // A screen that fires several requests gets several rejections at once.
        // Refreshing once per rejection would spend the rotating token repeatedly,
        // and the server answers a re-used token by killing the session — so the
        // second caller has to wait for the first and take its result.
        val reachedServer = CountDownLatch(1)
        val letServerAnswer = CountDownLatch(1)
        val renewals = AtomicInteger()
        val gateway =
            object : AuthGateway by FakeAuthGateway() {
                override suspend fun renew(refreshToken: String): SessionDto {
                    val attempt = renewals.incrementAndGet()
                    reachedServer.countDown()
                    letServerAnswer.await(5, TimeUnit.SECONDS)
                    return sessionOf("access-$attempt", "refresh-$attempt")
                }
            }
        val session =
            SessionManager(
                gateway = gateway,
                accessTokens = SessionTokens(),
                refreshTokens = FakeRefreshTokenStore("refresh-0"),
                serverAddresses = FakeServerAddressStore(ADDRESS),
                serverUrl = ServerUrl(ADDRESS),
                replica = FakeLocalReplica(),
                networks = FakeNetworkAvailability(),
                cleartext = CleartextPolicy(allowed = false),
                scope = CoroutineScope(Dispatchers.Unconfined),
            )

        val first = thread { session.renewAccessToken("access-0") }
        assertTrue(reachedServer.await(5, TimeUnit.SECONDS))
        val second = thread { session.renewAccessToken("access-0") }
        letServerAnswer.countDown()
        first.join(5_000)
        second.join(5_000)

        assertEquals(1, renewals.get(), "the second caller must not spend the rotated token again")
    }

    // --- signing in -------------------------------------------------------

    @Test
    fun `connecting refuses a plain http address without contacting anything`() =
        runTest {
            val gateway = FakeAuthGateway()
            val session = manager(gateway = gateway)

            val result = session.connect("http://todo.example.com")

            assertEquals(AuthResult.Failed(AuthFailure.NotEncrypted), result)
            assertEquals(AuthStep.Connect, session.authStep.value)
        }

    @Test
    fun `an address that does not answer is not remembered`() =
        runTest {
            val gateway = FakeAuthGateway().apply { reachable = false }
            val addresses = FakeServerAddressStore()
            val serverUrl = ServerUrl()
            val session = manager(gateway = gateway, addresses = addresses, serverUrl = serverUrl)

            val result = session.connect("https://wrong.example.com")

            assertEquals(AuthResult.Failed(AuthFailure.Unreachable), result)
            assertNull(addresses.stored())
            assertFalse(serverUrl.isConfigured, "a failed attempt must not leave the app pointed at nothing")
        }

    @Test
    fun `a mistyped change of server leaves the working one in place`() =
        runTest {
            val gateway = FakeAuthGateway()
            val serverUrl = ServerUrl(ADDRESS)
            val session = manager(gateway = gateway, serverUrl = serverUrl)
            gateway.reachable = false

            session.connect("https://typo.example.com")

            assertEquals(ADDRESS, serverUrl.value.toString())
        }

    @Test
    fun `connecting to a server with an account leads to the sign-in screen`() =
        runTest {
            val addresses = FakeServerAddressStore()
            val session = manager(addresses = addresses)

            val result = session.connect("todo.example.com")

            assertEquals(AuthResult.MovedTo(AuthStep.SignIn), result)
            assertEquals(ADDRESS, addresses.stored())
        }

    @Test
    fun `an account with a second factor is not signed in until the code is right`() =
        runTest {
            val gateway = FakeAuthGateway()
            gateway.signInResult = { LoginOutcome.OtpRequired("ticket-1") }
            val tokens = FakeRefreshTokenStore()
            val session = manager(gateway = gateway, tokens = tokens, addresses = FakeServerAddressStore(ADDRESS))

            assertEquals(AuthResult.OtpRequired, session.signIn("alice", "secret"))
            assertEquals(SessionState.Connecting, session.state.value)
            assertNull(tokens.stored())

            assertEquals(AuthResult.Done, session.verifyOtp("123456"))
            assertEquals(SessionState.LoggedIn, session.state.value)
            assertEquals("refresh-otp", tokens.stored())
        }

    @Test
    fun `signing in to a server that has no account yet moves to setup instead of failing`() =
        runTest {
            val gateway = FakeAuthGateway()
            gateway.signInResult = { throw ApiException.SetupRequired("setup needed") }
            val session = manager(gateway = gateway, addresses = FakeServerAddressStore(ADDRESS))

            val result = session.signIn("alice", "secret")

            assertEquals(AuthResult.MovedTo(AuthStep.Setup), result)
            assertEquals(AuthStep.Setup, session.authStep.value)
        }

    @Test
    fun `a wrong password is reported without ending anything`() =
        runTest {
            val gateway = FakeAuthGateway()
            gateway.signInResult = { throw rejection() }
            val session = manager(gateway = gateway, addresses = FakeServerAddressStore(ADDRESS))

            assertEquals(AuthResult.Failed(AuthFailure.BadCredentials), session.signIn("alice", "wrong"))
        }

    // --- signing out ------------------------------------------------------

    @Test
    fun `signing out with nothing queued asks nothing and empties the device`() =
        runTest {
            val gateway = FakeAuthGateway()
            val tokens = FakeRefreshTokenStore("refresh-1")
            val replica = FakeLocalReplica(unsent = 0)
            var asked = false
            val session =
                manager(
                    gateway = gateway,
                    tokens = tokens,
                    addresses = FakeServerAddressStore(ADDRESS),
                    replica = replica,
                )

            val signedOut =
                session.logOut {
                    asked = true
                    true
                }

            assertTrue(signedOut)
            assertFalse(asked, "there is nothing to lose, so there is nothing to ask about")
            assertEquals(1, gateway.revocations)
            assertNull(tokens.stored())
            assertEquals(1, replica.wipes)
            assertEquals(SessionState.LoggedOut, session.state.value)
        }

    @Test
    fun `signing out asks before discarding queued work, and declining changes nothing`() =
        runTest {
            val gateway = FakeAuthGateway()
            val tokens = FakeRefreshTokenStore("refresh-1")
            val replica = FakeLocalReplica(unsent = 4)
            val session =
                manager(
                    gateway = gateway,
                    tokens = tokens,
                    addresses = FakeServerAddressStore(ADDRESS),
                    replica = replica,
                )
            var offered = 0

            val signedOut =
                session.logOut { count ->
                    offered = count
                    false
                }

            assertFalse(signedOut)
            assertEquals(4, offered, "the user is shown the number they are agreeing to lose")
            assertEquals(0, gateway.revocations)
            assertEquals("refresh-1", tokens.stored())
            assertEquals(0, replica.wipes)
        }

    @Test
    fun `signing out discards queued work once the user agrees`() =
        runTest {
            val tokens = FakeRefreshTokenStore("refresh-1")
            val replica = FakeLocalReplica(unsent = 4)
            val session =
                manager(tokens = tokens, addresses = FakeServerAddressStore(ADDRESS), replica = replica)

            assertTrue(session.logOut { true })

            assertNull(tokens.stored())
            assertEquals(1, replica.wipes)
        }

    @Test
    fun `signing out everywhere closes every session and empties this device too`() =
        runTest {
            val gateway = FakeAuthGateway()
            val tokens = FakeRefreshTokenStore("refresh-1")
            val replica = FakeLocalReplica(unsent = 2)
            val session =
                manager(
                    gateway = gateway,
                    tokens = tokens,
                    addresses = FakeServerAddressStore(ADDRESS),
                    replica = replica,
                )
            var offered = 0

            val signedOut =
                session.logOutEverywhere { count ->
                    offered = count
                    true
                }

            assertTrue(signedOut)
            // It costs this device exactly what an ordinary sign-out costs, so it
            // asks the same question with the same number in it.
            assertEquals(2, offered)
            assertEquals(1, gateway.everySessionRevocations)
            assertNull(tokens.stored())
            assertEquals(1, replica.wipes)
            assertEquals(SessionState.LoggedOut, session.state.value)
        }

    @Test
    fun `declining the question leaves every session alone, this one included`() =
        runTest {
            val gateway = FakeAuthGateway()
            val tokens = FakeRefreshTokenStore("refresh-1")
            val replica = FakeLocalReplica(unsent = 2)
            val session =
                manager(
                    gateway = gateway,
                    tokens = tokens,
                    addresses = FakeServerAddressStore(ADDRESS),
                    replica = replica,
                )

            assertFalse(session.logOutEverywhere { false })

            assertEquals(0, gateway.everySessionRevocations)
            assertEquals("refresh-1", tokens.stored())
            assertEquals(0, replica.wipes)
        }

    @Test
    fun `signing out works when the server cannot be told`() =
        runTest {
            // A device that cannot reach its server must still be able to sign out
            // of it; the session dies with the refresh token in any case.
            val gateway = FakeAuthGateway().apply { revokeFails = true }
            val tokens = FakeRefreshTokenStore("refresh-1")
            val replica = FakeLocalReplica()
            val session =
                manager(
                    gateway = gateway,
                    tokens = tokens,
                    addresses = FakeServerAddressStore(ADDRESS),
                    replica = replica,
                )

            assertTrue(session.logOut { true })

            assertNull(tokens.stored())
            assertEquals(1, replica.wipes)
            assertEquals(SessionState.LoggedOut, session.state.value)
        }

    @Test
    fun `signing out keeps the server address, which is configuration rather than a credential`() =
        runTest {
            val addresses = FakeServerAddressStore(ADDRESS)
            val serverUrl = ServerUrl(ADDRESS)
            val session = manager(addresses = addresses, serverUrl = serverUrl)

            session.logOut { true }

            assertEquals(ADDRESS, addresses.stored())
            assertTrue(serverUrl.isConfigured)
            assertEquals(AuthStep.SignIn, session.authStep.value, "signing back in must not start by typing a URL")
        }

    @Test
    fun `disconnecting forgets the server as well as the session, and empties the device`() =
        runTest {
            // Stronger than signing out on purpose. A replica is a copy of one
            // installation's data and an id in it means nothing anywhere else, so
            // pointing the app at another server has to start from nothing.
            val tokens = FakeRefreshTokenStore("refresh-1")
            val addresses = FakeServerAddressStore(ADDRESS)
            val serverUrl = ServerUrl(ADDRESS)
            val replica = FakeLocalReplica(unsent = 2)
            val session =
                manager(tokens = tokens, addresses = addresses, serverUrl = serverUrl, replica = replica)

            session.disconnect()

            assertNull(tokens.stored())
            assertEquals(1, replica.wipes)
            assertNull(addresses.stored())
            assertFalse(serverUrl.isConfigured)
            assertEquals(SessionState.LoggedOut, session.state.value)
            assertEquals(AuthStep.Connect, session.authStep.value, "the app comes back at the address screen")
        }

    @Test
    fun `disconnecting works when the server cannot be told`() =
        runTest {
            val gateway = FakeAuthGateway().apply { revokeFails = true }
            val addresses = FakeServerAddressStore(ADDRESS)
            val session = manager(gateway = gateway, addresses = addresses, serverUrl = ServerUrl(ADDRESS))

            session.disconnect()

            assertNull(addresses.stored())
            assertEquals(AuthStep.Connect, session.authStep.value)
        }
}
