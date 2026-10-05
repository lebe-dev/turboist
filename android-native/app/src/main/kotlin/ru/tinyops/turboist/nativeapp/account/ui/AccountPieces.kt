package ru.tinyops.turboist.nativeapp.account.ui

import android.content.ClipData
import android.content.ClipDescription
import android.content.ClipboardManager
import android.content.Context
import android.os.Build
import android.os.PersistableBundle
import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.account.AccountProblem

/**
 * What the account screens show instead of a list they could not fetch.
 *
 * The same panel on all three, because they share the same honest limitation:
 * sessions, tokens and the second factor are read from the server as the user
 * looks at them and are kept nowhere, so with no connection there is nothing to
 * show. Saying that is the whole point — an empty list would answer "nothing can
 * reach your account", which is a very different statement and might be false.
 */
@Composable
fun RequiresConnection(
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier.fillMaxWidth().padding(vertical = 24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = stringResource(R.string.native_account_needsConnection),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
        TextButton(onClick = onRetry) { Text(stringResource(R.string.offline_retry)) }
    }
}

/**
 * One sentence for a refusal.
 *
 * Three of the four answers say the same thing wherever they happen — the phone
 * is offline, the deployment does not offer this, the code was wrong — so they
 * are worded once here. Only "the server refused" depends on what was being
 * attempted, which is why the caller supplies that one.
 */
@Composable
private fun accountProblemText(
    problem: AccountProblem,
    @StringRes refusedRes: Int,
): String =
    stringResource(
        when (problem) {
            AccountProblem.OFFLINE -> R.string.native_account_needsConnection
            AccountProblem.UNAVAILABLE -> R.string.settings_twofa_unavailable
            AccountProblem.INVALID_CODE -> R.string.settings_twofa_invalidCode
            AccountProblem.REFUSED -> refusedRes
        },
    )

/** Shows why the last attempt failed, or nothing when the last one worked. */
@Composable
fun AccountProblemText(
    problem: AccountProblem?,
    @StringRes refusedRes: Int,
    modifier: Modifier = Modifier,
) {
    if (problem == null) return
    Text(
        text = accountProblemText(problem, refusedRes),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.error,
        modifier = modifier,
    )
}

/**
 * Copies a secret to the clipboard, marked as one.
 *
 * The flag matters on these screens specifically: without it the system draws a
 * preview of what was copied, which would put a freshly minted API token or a
 * recovery code on screen again — over whatever the user switched to next, and
 * in front of whoever is standing behind them.
 */
@Composable
fun rememberSecretCopier(): (String) -> Unit {
    val context = LocalContext.current
    return remember(context) { { secret -> copySecret(context, secret) } }
}

private fun copySecret(
    context: Context,
    secret: String,
) {
    val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager ?: return
    val clip = ClipData.newPlainText(null, secret)
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        clip.description.extras =
            PersistableBundle().apply { putBoolean(ClipDescription.EXTRA_IS_SENSITIVE, true) }
    }
    clipboard.setPrimaryClip(clip)
}
