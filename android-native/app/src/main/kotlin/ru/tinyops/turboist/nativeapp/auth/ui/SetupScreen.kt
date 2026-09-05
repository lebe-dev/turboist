package ru.tinyops.turboist.nativeapp.auth.ui

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
import androidx.compose.ui.tooling.preview.Preview
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/**
 * Creates the one account the server hosts.
 *
 * Offered only when the server says it has none: the endpoint succeeds exactly
 * once per installation, so showing this screen to anyone else would be an
 * invitation to a refusal.
 */
@Composable
fun SetupScreen(
    state: AuthUiState,
    onCreateAccount: (String, String, String) -> Unit,
    onEdit: () -> Unit,
    onChangeServer: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var confirmation by remember { mutableStateOf("") }

    AuthScaffold(
        titleRes = R.string.auth_setupTitle,
        subtitleRes = R.string.auth_setupSubtitle,
        modifier = modifier,
    ) {
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
        )
        AuthField(
            value = confirmation,
            onValueChange = {
                confirmation = it
                onEdit()
            },
            labelRes = R.string.auth_confirmPassword,
            enabled = !state.busy,
            secret = true,
            imeAction = ImeAction.Done,
        )
        AuthFailureText(state.failure)
        AuthSubmit(
            labelRes = R.string.auth_createAccount,
            busyLabelRes = R.string.auth_creating,
            busy = state.busy,
            enabled = username.isNotBlank() && password.isNotBlank() && confirmation.isNotBlank(),
            onClick = { onCreateAccount(username, password, confirmation) },
        )
        TextButton(onClick = onChangeServer, enabled = !state.busy) {
            Text(stringResource(R.string.native_connect_change_server))
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun SetupScreenPreview() {
    TurboistTheme {
        SetupScreen(
            state = AuthUiState(),
            onCreateAccount = { _, _, _ -> },
            onEdit = {},
            onChangeServer = {},
        )
    }
}
