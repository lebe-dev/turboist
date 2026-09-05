package ru.tinyops.turboist.nativeapp.auth.ui

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.auth.AuthFailure

/**
 * The frame every sign-in screen sits in: a title, a sentence of explanation and
 * a single column of fields, centred and capped in width so the form does not
 * stretch across a tablet.
 */
@Composable
fun AuthScaffold(
    @StringRes titleRes: Int,
    @StringRes subtitleRes: Int?,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    Column(
        modifier =
            modifier
                .fillMaxSize()
                .safeDrawingPadding()
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 24.dp, vertical = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Column(
            modifier = Modifier.widthIn(max = 420.dp).fillMaxWidth(),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                text = stringResource(titleRes),
                style = MaterialTheme.typography.headlineSmall,
                color = MaterialTheme.colorScheme.onSurface,
                textAlign = TextAlign.Center,
            )
            if (subtitleRes != null) {
                Text(
                    text = stringResource(subtitleRes),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(top = 8.dp, bottom = 8.dp),
                )
            }
            content()
        }
    }
}

/** A single-line field, locked while a call is in flight. */
@Composable
fun AuthField(
    value: String,
    onValueChange: (String) -> Unit,
    @StringRes labelRes: Int,
    enabled: Boolean,
    modifier: Modifier = Modifier,
    keyboardType: KeyboardType = KeyboardType.Text,
    secret: Boolean = false,
    imeAction: ImeAction = ImeAction.Next,
) {
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        label = { Text(stringResource(labelRes)) },
        enabled = enabled,
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = keyboardType, imeAction = imeAction),
        visualTransformation = if (secret) PasswordVisualTransformation() else VisualTransformation.None,
        modifier = modifier.fillMaxWidth().padding(top = 12.dp),
    )
}

/**
 * The screen's one action.
 *
 * The label changes while the call is out rather than the button disappearing:
 * "Signing in…" in the place the user just pressed is the clearest possible
 * answer to "did that do anything?".
 */
@Composable
fun AuthSubmit(
    @StringRes labelRes: Int,
    @StringRes busyLabelRes: Int,
    busy: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Button(
        onClick = onClick,
        enabled = enabled && !busy,
        modifier = modifier.fillMaxWidth().padding(top = 20.dp),
    ) {
        if (busy) {
            CircularProgressIndicator(
                modifier = Modifier.padding(end = 12.dp).size(16.dp),
                strokeWidth = 2.dp,
            )
        }
        Text(stringResource(if (busy) busyLabelRes else labelRes))
    }
}

/** The sentence explaining the last refusal. Draws nothing when there was none. */
@Composable
fun AuthFailureText(
    failure: AuthFailure?,
    modifier: Modifier = Modifier,
) {
    if (failure == null) return
    Text(
        text = stringResource(failure.messageRes()),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.error,
        textAlign = TextAlign.Center,
        modifier = modifier.fillMaxWidth().padding(top = 16.dp),
    )
}

/**
 * The sentence each refusal is shown as.
 *
 * Wording the web client also uses comes from the shared locale files, so the
 * two clients cannot drift apart on what "we could not reach that server"
 * means; only the cases the web has no screen for are worded here.
 */
@StringRes
fun AuthFailure.messageRes(): Int =
    when (this) {
        AuthFailure.Unreadable -> R.string.connect_invalidUrl
        AuthFailure.NotEncrypted -> R.string.native_connect_requires_https
        AuthFailure.Unreachable -> R.string.connect_unreachable
        AuthFailure.BadCredentials -> R.string.auth_loginFailed
        AuthFailure.InvalidCode -> R.string.auth_otpFailed
        AuthFailure.TooManyAttempts -> R.string.native_auth_too_many_attempts
        AuthFailure.PasswordsDoNotMatch -> R.string.auth_passwordsMismatch
        AuthFailure.AlreadySetUp -> R.string.native_auth_already_set_up
        AuthFailure.InvalidDetails -> R.string.native_auth_invalid_details
        AuthFailure.ServerError -> R.string.native_auth_server_error
    }
