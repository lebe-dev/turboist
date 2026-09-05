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
import ru.tinyops.turboist.core.network.dto.ApiTokenDto
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * Minting and revoking the tokens external tools authenticate with.
 *
 * The rules worth pinning are about the plaintext and about the grant.
 *
 * The plaintext exists in one response and nowhere else — the server kept only
 * a hash — so it lives in memory for as long as the dialog showing it and is
 * dropped with it. It never joins the list, which is what the screen reads from.
 *
 * The grant is fixed for the life of the token, so a selection the server would
 * refuse has to be impossible to submit rather than reported afterwards.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class ApiTokensViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val control = FakeApiTokenControl()

    @Before
    fun useTestDispatcher() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun releaseDispatcher() {
        Dispatchers.resetMain()
    }

    private fun tokenOf(
        id: Long,
        name: String,
    ) = ApiTokenDto(id = id, name = name, scopes = listOf("tasks:read"), createdAt = "2026-08-18T10:00:00.000Z")

    @Test
    fun `the tokens arrive and the screen stops waiting`() =
        runTest(dispatcher) {
            control.stored += listOf(tokenOf(1, "n8n"), tokenOf(2, "backup"))

            val model = ApiTokensViewModel(control)
            advanceUntilIdle()

            assertFalse(model.state.value.loading)
            assertEquals(listOf("n8n", "backup"), model.state.value.tokens.map { it.name })
        }

    @Test
    fun `a list that could not be fetched is said to be missing, not empty`() =
        runTest(dispatcher) {
            control.listFailure = offline()

            val model = ApiTokensViewModel(control)
            advanceUntilIdle()

            assertTrue(model.state.value.unreachable)
            assertEquals(AccountProblem.OFFLINE, model.state.value.problem)
            assertTrue(model.state.value.tokens.isEmpty())
        }

    @Test
    fun `the plaintext is shown once and then forgotten`() =
        runTest(dispatcher) {
            val model = ApiTokensViewModel(control)
            advanceUntilIdle()

            model.editName("  n8n  ")
            model.setWrite(ScopeResource.TASKS, true)
            model.create()
            advanceUntilIdle()

            val minted = assertNotNull(model.state.value.created)
            assertEquals("plaintext-n8n", minted.token)
            // The name travels trimmed, and the grant is the one the form built.
            assertEquals(listOf("n8n" to listOf("tasks:read", "tasks:write")), control.created)

            // The listed form of the same token carries no secret at all, which
            // is what the screen reads from once the dialog is closed.
            val listed = model.state.value.tokens.single()
            assertEquals("n8n", listed.name)
            assertEquals(listOf("tasks:read", "tasks:write"), listed.scopes)

            model.dismissCreated()

            // Gone: the server cannot reissue it, and keeping it would be a copy
            // of a credential with nowhere safe to live.
            assertNull(model.state.value.created)
        }

    @Test
    fun `the form is emptied after a token is minted`() =
        runTest(dispatcher) {
            val model = ApiTokensViewModel(control)
            advanceUntilIdle()

            model.editName("n8n")
            model.applyPreset(ScopePreset.READ_ONLY)
            model.create()
            advanceUntilIdle()

            // Leaving the previous grant ticked would make the next token a copy
            // of the last one by accident.
            assertEquals("", model.state.value.name)
            assertTrue(model.state.value.selection.isEmpty)
        }

    @Test
    fun `a token with no permissions is refused here rather than by the server`() =
        runTest(dispatcher) {
            val model = ApiTokensViewModel(control)
            advanceUntilIdle()

            model.editName("n8n")
            model.create()
            advanceUntilIdle()

            assertTrue(model.state.value.scopesMissing)
            assertTrue(control.created.isEmpty())

            // Ticking anything clears the complaint without a round trip.
            model.setRead(ScopeResource.TASKS, true)
            assertFalse(model.state.value.scopesMissing)
        }

    @Test
    fun `a token with no name is not sent`() =
        runTest(dispatcher) {
            val model = ApiTokensViewModel(control)
            advanceUntilIdle()

            model.editName("   ")
            model.applyPreset(ScopePreset.FULL_ACCESS)
            model.create()
            advanceUntilIdle()

            assertTrue(control.created.isEmpty())
            assertFalse(model.state.value.canCreate)
        }

    @Test
    fun `deleting asks first, and a refusal leaves the token where it is`() =
        runTest(dispatcher) {
            control.stored += listOf(tokenOf(1, "n8n"), tokenOf(2, "backup"))
            control.deleteFailure = refusal()
            val model = ApiTokensViewModel(control)
            advanceUntilIdle()

            model.askDelete(model.state.value.tokens.first())
            advanceUntilIdle()
            assertNotNull(model.state.value.deleting)
            assertTrue(control.deleted.isEmpty())

            model.confirmDelete()
            advanceUntilIdle()

            // A row dropped on a failed call would claim an integration is
            // locked out while it is still working.
            assertEquals(listOf("n8n", "backup"), model.state.value.tokens.map { it.name })
            assertEquals(AccountProblem.REFUSED, model.state.value.problem)

            control.deleteFailure = null
            model.askDelete(model.state.value.tokens.first())
            model.confirmDelete()
            advanceUntilIdle()

            assertEquals(listOf(1L), control.deleted)
            assertEquals(listOf("backup"), model.state.value.tokens.map { it.name })
        }

    @Test
    fun `backing out of a deletion changes nothing`() =
        runTest(dispatcher) {
            control.stored += tokenOf(1, "n8n")
            val model = ApiTokensViewModel(control)
            advanceUntilIdle()

            model.askDelete(model.state.value.tokens.single())
            model.cancelDelete()
            advanceUntilIdle()

            assertNull(model.state.value.deleting)
            assertTrue(control.deleted.isEmpty())
        }
}
