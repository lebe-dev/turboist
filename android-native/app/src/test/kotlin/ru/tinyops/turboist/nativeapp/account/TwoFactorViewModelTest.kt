package ru.tinyops.turboist.nativeapp.account

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Before
import org.junit.Test
import ru.tinyops.turboist.core.network.ApiErrorCodes
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * Turning the time-based second factor on and off.
 *
 * What is pinned here is the handling of the two secrets and the one deployment
 * difference.
 *
 * The pending secret is live on the server the moment it is issued, so it is
 * dropped as soon as it has been proved — and dropped just as completely when
 * the user walks away, which leaves the account exactly as it was.
 *
 * The recovery codes are readable once. They go when the user says they are
 * done, because there is nowhere for them to be kept: the server holds only
 * their hashes.
 *
 * And a deployment that was never configured for a second factor has no such
 * endpoints. Its refusal is an answer — "not offered here" — rather than a
 * failure to retry, so the screen stops offering instead of inviting another tap.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class TwoFactorViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val control = FakeTwoFactorControl()

    @Before
    fun useTestDispatcher() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun releaseDispatcher() {
        Dispatchers.resetMain()
    }

    @Test
    fun `whether the second factor is on is read from the server`() =
        runTest(dispatcher) {
            control.enabled = true

            val model = TwoFactorViewModel(control)
            advanceUntilIdle()

            assertFalse(model.state.value.loading)
            assertTrue(model.state.value.enabled)
            assertTrue(model.state.value.available)
        }

    @Test
    fun `a status that could not be read is said to be unknown, not off`() =
        runTest(dispatcher) {
            control.statusFailure = offline()

            val model = TwoFactorViewModel(control)
            advanceUntilIdle()

            // "Off" would invite the user to enrol a second factor they may
            // already have, on an account whose state nobody could check.
            assertTrue(model.state.value.unreachable)
            assertEquals(AccountProblem.OFFLINE, model.state.value.problem)
            assertFalse(model.state.value.enabled)
        }

    @Test
    fun `enrolling issues a secret, proves it, and hands back the recovery codes`() =
        runTest(dispatcher) {
            val model = TwoFactorViewModel(control)
            advanceUntilIdle()

            model.beginEnrolment()
            advanceUntilIdle()

            assertEquals(TwoFactorStep.ENROLLING, model.state.value.step)
            assertEquals("JBSWY3DPEHPK3PXP", assertNotNull(model.state.value.enrolment).secret)
            // Nothing is on yet: an enrolment stops here until a code proves it.
            assertFalse(model.state.value.enabled)

            model.editCode("  123456  ")
            model.confirmEnrolment()
            advanceUntilIdle()

            assertEquals(listOf("123456"), control.confirmedCodes)
            assertTrue(model.state.value.enabled)
            assertEquals(TwoFactorStep.RECOVERY, model.state.value.step)
            assertEquals(listOf("AAAA1111", "BBBB2222"), model.state.value.recoveryCodes)
            // The secret has done its job and is gone.
            assertNull(model.state.value.enrolment)
        }

    @Test
    fun `the recovery codes are dropped once the user says they are done`() =
        runTest(dispatcher) {
            val model = TwoFactorViewModel(control)
            advanceUntilIdle()
            model.beginEnrolment()
            advanceUntilIdle()
            model.editCode("123456")
            model.confirmEnrolment()
            advanceUntilIdle()

            model.finishRecovery()

            // There is nowhere for them to go: the server kept only hashes, so
            // holding them here would be the only copy and an unasked-for one.
            assertTrue(model.state.value.recoveryCodes.isEmpty())
            assertEquals(TwoFactorStep.IDLE, model.state.value.step)
        }

    @Test
    fun `walking away from an enrolment drops the secret and leaves the account alone`() =
        runTest(dispatcher) {
            val model = TwoFactorViewModel(control)
            advanceUntilIdle()
            model.beginEnrolment()
            advanceUntilIdle()

            model.cancelEnrolment()

            assertNull(model.state.value.enrolment)
            assertEquals(TwoFactorStep.IDLE, model.state.value.step)
            assertFalse(model.state.value.enabled)
            assertTrue(control.confirmedCodes.isEmpty())
        }

    @Test
    fun `a wrong code is said to be a wrong code, and the enrolment stays open`() =
        runTest(dispatcher) {
            control.confirmFailure = refusal(status = 401, code = ApiErrorCodes.TOTP_INVALID_CODE)
            val model = TwoFactorViewModel(control)
            advanceUntilIdle()
            model.beginEnrolment()
            advanceUntilIdle()

            model.editCode("000000")
            model.confirmEnrolment()
            advanceUntilIdle()

            assertEquals(AccountProblem.INVALID_CODE, model.state.value.problem)
            // Still on the enrolment step with the secret in hand: a mistyped
            // code costs a retype, not a new secret and a new QR code.
            assertEquals(TwoFactorStep.ENROLLING, model.state.value.step)
            assertNotNull(model.state.value.enrolment)
            assertFalse(model.state.value.enabled)
        }

    @Test
    fun `switching it off takes a current code or a recovery code`() =
        runTest(dispatcher) {
            control.enabled = true
            val model = TwoFactorViewModel(control)
            advanceUntilIdle()

            model.startDisabling()
            model.editCode("AAAA1111")
            model.disable()
            advanceUntilIdle()

            assertEquals(listOf("AAAA1111"), control.disableCodes)
            assertFalse(model.state.value.enabled)
            assertEquals(TwoFactorStep.IDLE, model.state.value.step)
            assertEquals(TwoFactorMessage.DISABLED, model.state.value.message)
        }

    @Test
    fun `a deployment without a second factor says so instead of failing`() =
        runTest(dispatcher) {
            control.beginFailure = absentRoute()
            val model = TwoFactorViewModel(control)
            advanceUntilIdle()

            model.beginEnrolment()
            advanceUntilIdle()

            // Nothing to retry: the routes are not there and no tap will bring
            // them back, so the screen stops offering rather than nagging.
            assertFalse(model.state.value.available)
            assertEquals(AccountProblem.UNAVAILABLE, model.state.value.problem)
            assertEquals(TwoFactorStep.IDLE, model.state.value.step)
            assertNull(model.state.value.enrolment)
        }

    @Test
    fun `a second call cannot start while the first is out`() =
        runTest(dispatcher) {
            val model = TwoFactorViewModel(control)
            advanceUntilIdle()

            model.beginEnrolment()
            model.beginEnrolment()
            advanceUntilIdle()

            assertEquals(1, control.beginCalls, "a second secret was issued over the first")
        }
}
