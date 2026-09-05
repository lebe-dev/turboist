package ru.tinyops.turboist.nativeapp.navigation

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import ru.tinyops.turboist.nativeapp.auth.AuthStep
import ru.tinyops.turboist.nativeapp.auth.ui.AuthViewModel
import ru.tinyops.turboist.nativeapp.auth.ui.ConnectScreen
import ru.tinyops.turboist.nativeapp.auth.ui.SetupScreen
import ru.tinyops.turboist.nativeapp.auth.ui.SignInScreen

/**
 * The graph shown before there is a usable session: pick a server, then either
 * create the single account the server hosts or sign in to it.
 *
 * Which of the two follows the server screen is not a choice offered to the
 * user — only the server knows whether it has an account yet — so the flow is
 * driven by the session layer's answer rather than by links between the
 * screens. That also covers the case a link could not: signing in to an
 * instance that turns out to have no account moves to the setup screen instead
 * of showing a refusal the user could do nothing about.
 *
 * One view model backs all three destinations, so the second-factor step of a
 * sign-in survives anything the graph does around it.
 */
@Composable
fun AuthNavHost(
    navController: NavHostController,
    startStep: AuthStep,
    modifier: Modifier = Modifier,
    viewModel: AuthViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val step by viewModel.step.collectAsStateWithLifecycle()
    val passkeyOffered by viewModel.passkeyOffered.collectAsStateWithLifecycle()

    // The graph starts on whatever step was current when it appeared, and follows
    // later changes. Each move replaces the flow rather than stacking on it: a
    // back press out of sign-in must not land on a server screen the user has
    // already answered.
    var shown by remember { mutableStateOf(startStep) }
    LaunchedEffect(step) {
        if (step == shown) return@LaunchedEffect
        shown = step
        navController.navigate(routeFor(step)) {
            popUpTo(navController.graph.startDestinationId) { inclusive = true }
            launchSingleTop = true
        }
    }

    NavHost(
        navController = navController,
        startDestination = routeFor(startStep),
        modifier = modifier,
    ) {
        composable<ConnectRoute> {
            ConnectScreen(
                state = state,
                onConnect = viewModel::connect,
                onEdit = viewModel::clearFailure,
            )
        }
        composable<SetupRoute> {
            SetupScreen(
                state = state,
                onCreateAccount = viewModel::createAccount,
                onEdit = viewModel::clearFailure,
                onChangeServer = viewModel::changeServer,
            )
        }
        composable<LoginRoute> {
            SignInScreen(
                state = state,
                passkeyOffered = passkeyOffered,
                onSignIn = viewModel::signIn,
                onPasskeySignIn = viewModel::signInWithPasskey,
                onVerifyOtp = viewModel::verifyOtp,
                onCancelOtp = viewModel::cancelOtp,
                onUseRecoveryCode = viewModel::useRecoveryCode,
                onEdit = viewModel::clearFailure,
                onChangeServer = viewModel::changeServer,
            )
        }
    }
}

/** The destination each step of the sign-in flow is drawn at. */
private fun routeFor(step: AuthStep): Any =
    when (step) {
        AuthStep.Connect -> ConnectRoute
        AuthStep.Setup -> SetupRoute
        AuthStep.SignIn -> LoginRoute
    }
