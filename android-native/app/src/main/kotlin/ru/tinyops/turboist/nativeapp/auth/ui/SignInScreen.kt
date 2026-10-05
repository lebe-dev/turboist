package ru.tinyops.turboist.nativeapp.auth.ui

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyAuthenticator
import ru.tinyops.turboist.nativeapp.auth.passkey.ui.PasskeyProblemText
import ru.tinyops.turboist.nativeapp.auth.passkey.ui.rememberPasskeyAuthenticator
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/**
 * Signing in to an account that already exists.
 *
 * The second factor is a step of this screen rather than a screen of its own,
 * because it is a continuation of the same attempt: the ticket the server hands
 * back is short-lived and single-use, so leaving the screen abandons it, and a
 * separate destination would invite exactly that.
 *
 * A passkey, when the server has one to offer, is a second way in rather than a
 * replacement: the password form stays exactly where it was, because it is the
 * recovery path for every case a passkey cannot cover — a new phone, a lost
 * credential, a server whose association files are not in place yet.
 */
@Composable
fun SignInScreen(
    state: AuthUiState,
    passkeyOffered: Boolean,
    onSignIn: (String, String) -> Unit,
    onPasskeySignIn: (PasskeyAuthenticator) -> Unit,
    onVerifyOtp: (String) -> Unit,
    onCancelOtp: () -> Unit,
    onUseRecoveryCode: (Boolean) -> Unit,
    onEdit: () -> Unit,
    onChangeServer: () -> Unit,
    modifier: Modifier = Modifier,
) {
    if (state.awaitingOtp) {
        OtpStep(
            state = state,
            onVerify = onVerifyOtp,
            onCancel = onCancelOtp,
            onUseRecoveryCode = onUseRecoveryCode,
            onEdit = onEdit,
            modifier = modifier,
        )
        return
    }

    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }

    AuthScaffold(titleRes = R.string.auth_signIn, subtitleRes = null, modifier = modifier) {
        AuthField(
            value = username,
            onValueChange = {
                username = it
                onEdit()
            },
            labelRes = R.string.auth_username,
            enabled = !state.busy,
        )
        AuthField(
            value = password,
            onValueChange = {
                password = it
                onEdit()
            },
            labelRes = R.string.auth_password,
            enabled = !state.busy,
            secret = true,
            imeAction = ImeAction.Done,
        )
        AuthFailureText(state.failure)
        AuthSubmit(
            labelRes = R.string.auth_signIn,
            busyLabelRes = R.string.auth_signingIn,
            busy = state.busy,
            enabled = username.isNotBlank() && password.isNotBlank(),
            onClick = { onSignIn(username, password) },
        )
        if (passkeyOffered) PasskeySignIn(state = state, onSignIn = onPasskeySignIn)
        TextButton(onClick = onChangeServer, enabled = !state.busy) {
            Text(stringResource(R.string.native_connect_change_server))
        }
    }
}

/**
 * The passkey alternative, below the password form and separated from it.
 *
 * The order is deliberate: the password is the one way in that always works, so
 * it keeps the top of the screen, and the passkey is offered as the shortcut it
 * is. Whatever goes wrong with a ceremony is said here, next to the button that
 * started it, and never replaces the form.
 */
@Composable
private fun PasskeySignIn(
    state: AuthUiState,
    onSignIn: (PasskeyAuthenticator) -> Unit,
) {
    val authenticator = rememberPasskeyAuthenticator()

    HorizontalDivider(modifier = Modifier.padding(top = 24.dp))
    Text(
        text = stringResource(R.string.auth_passkeyOr),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        textAlign = TextAlign.Center,
        modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
    )
    OutlinedButton(
        onClick = { onSignIn(authenticator) },
        enabled = !state.busy,
        modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
    ) {
        Text(
            stringResource(
                if (state.awaitingDevice) R.string.auth_passkeySigningIn else R.string.auth_passkeySignIn,
            ),
        )
    }
    PasskeyProblemText(state.passkeyProblem)
}

/**
 * The second factor.
 *
 * One field for both kinds of code: a recovery code goes to the same endpoint as
 * a code from the authenticator app, so the toggle changes the wording and the
 * keyboard and nothing else.
 */
@Composable
private fun OtpStep(
    state: AuthUiState,
    onVerify: (String) -> Unit,
    onCancel: () -> Unit,
    onUseRecoveryCode: (Boolean) -> Unit,
    onEdit: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var code by remember(state.usingRecoveryCode) { mutableStateOf("") }
    val recovery = state.usingRecoveryCode

    AuthScaffold(
        titleRes = R.string.auth_otpTitle,
        subtitleRes = if (recovery) R.string.auth_otpRecoveryHint else R.string.auth_otpAppHint,
        modifier = modifier,
    ) {
        AuthField(
            value = code,
            onValueChange = {
                code = it
                onEdit()
            },
            labelRes = if (recovery) R.string.auth_otpRecoveryLabel else R.string.auth_otpAppLabel,
            enabled = !state.busy,
            keyboardType = if (recovery) KeyboardType.Text else KeyboardType.NumberPassword,
            imeAction = ImeAction.Done,
        )
        AuthFailureText(state.failure)
        AuthSubmit(
            labelRes = R.string.auth_otpVerify,
            busyLabelRes = R.string.auth_signingIn,
            busy = state.busy,
            enabled = code.isNotBlank(),
            onClick = { onVerify(code) },
        )
        TextButton(onClick = { onUseRecoveryCode(!recovery) }, enabled = !state.busy) {
            Text(stringResource(if (recovery) R.string.auth_otpUseApp else R.string.auth_otpUseRecovery))
        }
        TextButton(onClick = onCancel, enabled = !state.busy) {
            Text(stringResource(R.string.auth_otpCancel))
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun SignInScreenPreview() {
    TurboistTheme {
        SignInScreen(
            state = AuthUiState(),
            passkeyOffered = true,
            onSignIn = { _, _ -> },
            onPasskeySignIn = {},
            onVerifyOtp = {},
            onCancelOtp = {},
            onUseRecoveryCode = {},
            onEdit = {},
            onChangeServer = {},
        )
    }
}

@Preview(showBackground = true)
@Composable
private fun OtpStepPreview() {
    TurboistTheme {
        SignInScreen(
            state = AuthUiState(awaitingOtp = true),
            passkeyOffered = false,
            onSignIn = { _, _ -> },
            onPasskeySignIn = {},
            onVerifyOtp = {},
            onCancelOtp = {},
            onUseRecoveryCode = {},
            onEdit = {},
            onChangeServer = {},
        )
    }
}
