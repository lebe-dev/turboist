package ru.tinyops.turboist.nativeapp.auth.passkey.ui

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.auth.passkey.CredentialManagerAuthenticator
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyAuthenticator
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyProblem

/**
 * The sentence each way of not getting a passkey is shown as.
 *
 * Every one of them names the password as the way forward, because it always is:
 * a passkey is an additional login method and deleting or failing to use one can
 * never lock the account out. A user who is told only that something failed will
 * try the same button again; one who is told to use their password gets in.
 */
@StringRes
fun PasskeyProblem.messageRes(): Int =
    when (this) {
        PasskeyProblem.Cancelled -> R.string.native_passkey_cancelled
        PasskeyProblem.NotEnrolled -> R.string.native_passkey_not_enrolled
        PasskeyProblem.Untrusted -> R.string.native_passkey_untrusted
        PasskeyProblem.Unsupported -> R.string.native_passkey_unsupported
        PasskeyProblem.Refused -> R.string.native_passkey_refused
        PasskeyProblem.AlreadyRegistered -> R.string.settings_passkeys_duplicate
        PasskeyProblem.LimitReached -> R.string.settings_passkeys_limitReached
        PasskeyProblem.Unreachable -> R.string.connect_unreachable
        PasskeyProblem.Failed -> R.string.auth_passkeyFailed
    }

/** The sentence explaining the last ceremony. Draws nothing when there was none. */
@Composable
fun PasskeyProblemText(
    problem: PasskeyProblem?,
    modifier: Modifier = Modifier,
) {
    if (problem == null) return
    Text(
        text = stringResource(problem.messageRes()),
        style = MaterialTheme.typography.bodyMedium,
        // A cancellation is a choice the user made, not a fault, so it is stated
        // rather than alarmed about.
        color =
            if (problem == PasskeyProblem.Cancelled) {
                MaterialTheme.colorScheme.onSurfaceVariant
            } else {
                MaterialTheme.colorScheme.error
            },
        textAlign = TextAlign.Center,
        modifier = modifier.fillMaxWidth().padding(top = 16.dp),
    )
}

/**
 * The device's authenticator, bound to the activity the ceremony will be drawn
 * over.
 *
 * Built here rather than injected: the system sheet belongs to the screen the
 * user is looking at, and the dependency graph has no notion of which one that
 * is. Nothing platform-specific happens until a ceremony actually starts, so a
 * preview may hold one safely.
 */
@Composable
fun rememberPasskeyAuthenticator(): PasskeyAuthenticator {
    val context = LocalContext.current
    return remember(context) { CredentialManagerAuthenticator(context) }
}
