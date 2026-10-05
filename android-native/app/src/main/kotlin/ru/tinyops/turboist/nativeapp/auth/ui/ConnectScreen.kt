package ru.tinyops.turboist.nativeapp.auth.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.tooling.preview.Preview
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/**
 * The first screen of a fresh install: which server does this app talk to?
 *
 * There is no default and no list to pick from — the product is self-hosted, so
 * only the user knows the address. It is checked against the server before it is
 * remembered, which is what turns a typo into a sentence here rather than into
 * an unexplained failure on the sign-in screen after it.
 */
@Composable
fun ConnectScreen(
    state: AuthUiState,
    onConnect: (String) -> Unit,
    onEdit: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var address by remember { mutableStateOf("") }

    AuthScaffold(
        titleRes = R.string.connect_title,
        subtitleRes = R.string.connect_subtitle,
        modifier = modifier,
    ) {
        AuthField(
            value = address,
            onValueChange = {
                address = it
                onEdit()
            },
            labelRes = R.string.native_connect_address,
            enabled = !state.busy,
            keyboardType = KeyboardType.Uri,
            imeAction = ImeAction.Go,
        )
        AuthFailureText(state.failure)
        AuthSubmit(
            labelRes = R.string.connect_connect,
            busyLabelRes = R.string.connect_checking,
            busy = state.busy,
            enabled = address.isNotBlank(),
            onClick = { onConnect(address) },
        )
    }
}

@Preview(showBackground = true)
@Composable
private fun ConnectScreenPreview() {
    TurboistTheme {
        ConnectScreen(state = AuthUiState(), onConnect = {}, onEdit = {})
    }
}
