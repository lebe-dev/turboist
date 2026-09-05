package ru.tinyops.turboist.nativeapp.account.ui

import android.graphics.BitmapFactory
import android.util.Base64
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import ru.tinyops.turboist.core.network.dto.TotpEnrolmentDto
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.account.TwoFactorStep
import ru.tinyops.turboist.nativeapp.account.TwoFactorUiState
import ru.tinyops.turboist.nativeapp.account.TwoFactorViewModel
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/** Everything the second-factor screen can be asked to do. */
data class TwoFactorCallbacks(
    val onBegin: () -> Unit = {},
    val onEditCode: (String) -> Unit = {},
    val onConfirm: () -> Unit = {},
    val onCancelEnrolment: () -> Unit = {},
    val onFinishRecovery: () -> Unit = {},
    val onStartDisabling: () -> Unit = {},
    val onCancelDisabling: () -> Unit = {},
    val onDisable: () -> Unit = {},
    val onRetry: () -> Unit = {},
)

/**
 * Turning the time-based second factor on and off.
 *
 * Read live: whether the account asks for a code is a question only the server
 * can answer, and it can be answered differently a minute later because another
 * device switched it. Nothing about it is stored here.
 *
 * Two values cross this screen and are deliberately not written anywhere. The
 * enrolment secret is live from the moment the server issues it, so a copy of it
 * on this phone would put the second factor in the same place as the first. The
 * recovery codes exist in one response and nowhere else — the server kept only
 * their hashes — so the screen says plainly that this is the only time they can
 * be read, and drops them when the user says they are done.
 */
@Composable
fun TwoFactorScreen(
    modifier: Modifier = Modifier,
    viewModel: TwoFactorViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    TwoFactorContent(
        state = state,
        callbacks =
            TwoFactorCallbacks(
                onBegin = viewModel::beginEnrolment,
                onEditCode = viewModel::editCode,
                onConfirm = viewModel::confirmEnrolment,
                onCancelEnrolment = viewModel::cancelEnrolment,
                onFinishRecovery = viewModel::finishRecovery,
                onStartDisabling = viewModel::startDisabling,
                onCancelDisabling = viewModel::cancelDisabling,
                onDisable = viewModel::disable,
                onRetry = viewModel::load,
            ),
        modifier = modifier,
    )
}

@Composable
internal fun TwoFactorContent(
    state: TwoFactorUiState,
    callbacks: TwoFactorCallbacks,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(
            text = stringResource(R.string.settings_twofa_description),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        if (!state.available) {
            // Nothing to offer and nothing to retry: this deployment was never
            // configured for a second factor, so the routes do not exist.
            Text(
                text = stringResource(R.string.settings_twofa_unavailable),
                style = MaterialTheme.typography.bodyMedium,
            )
            return@Column
        }

        if (state.unreachable) {
            // The panel says why there is nothing here; the reason above it
            // would be the same sentence twice.
            RequiresConnection(onRetry = callbacks.onRetry)
            return@Column
        }

        AccountProblemText(state.problem, R.string.settings_twofa_setupFailed)

        when (state.step) {
            TwoFactorStep.IDLE -> Status(state = state, callbacks = callbacks)
            TwoFactorStep.ENROLLING -> Enrolment(state = state, callbacks = callbacks)
            TwoFactorStep.RECOVERY -> RecoveryCodes(codes = state.recoveryCodes, onDone = callbacks.onFinishRecovery)
            TwoFactorStep.DISABLING -> DisableForm(state = state, callbacks = callbacks)
        }
    }
}

@Composable
private fun Status(
    state: TwoFactorUiState,
    callbacks: TwoFactorCallbacks,
) {
    if (state.enabled) {
        Text(
            text = stringResource(R.string.settings_twofa_statusEnabled),
            style = MaterialTheme.typography.titleMedium,
        )
        OutlinedButton(
            onClick = callbacks.onStartDisabling,
            enabled = !state.busy,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(stringResource(R.string.settings_twofa_disableButton))
        }
        return
    }
    Button(
        onClick = callbacks.onBegin,
        enabled = !state.busy && !state.loading,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Text(
            stringResource(if (state.busy) R.string.settings_twofa_starting else R.string.settings_twofa_enableButton),
        )
    }
}

/**
 * The pending secret, in the three forms an authenticator can take it: the
 * picture, the text, and the code that proves one of them was stored.
 */
@Composable
private fun Enrolment(
    state: TwoFactorUiState,
    callbacks: TwoFactorCallbacks,
) {
    val enrolment = state.enrolment ?: return
    val copy = rememberSecretCopier()
    var copied by remember(enrolment.secret) { mutableStateOf(false) }

    Text(text = stringResource(R.string.settings_twofa_setupHint), style = MaterialTheme.typography.bodyMedium)
    qrImage(enrolment)?.let { image ->
        Row(horizontalArrangement = Arrangement.Center, modifier = Modifier.fillMaxWidth()) {
            Image(
                bitmap = image,
                contentDescription = stringResource(R.string.settings_twofa_qrAlt),
                modifier = Modifier.size(QR_SIZE),
            )
        }
    }
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(12.dp)) {
            Text(
                text = stringResource(R.string.settings_twofa_secretLabel),
                style = MaterialTheme.typography.labelMedium,
            )
            Text(text = enrolment.secret, style = MaterialTheme.typography.bodyMedium)
            if (copied) {
                Text(
                    text = stringResource(R.string.settings_twofa_secretCopied),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
            TextButton(onClick = {
                copy(enrolment.secret)
                copied = true
            }) { Text(stringResource(R.string.settings_api_copy)) }
        }
    }
    CodeField(
        code = state.code,
        labelRes = R.string.settings_twofa_codePlaceholder,
        enabled = !state.busy,
        onEditCode = callbacks.onEditCode,
    )
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
        Button(
            onClick = callbacks.onConfirm,
            enabled = !state.busy && state.code.isNotBlank(),
            modifier = Modifier.weight(1f),
        ) {
            Text(
                stringResource(
                    if (state.busy) R.string.settings_twofa_confirming else R.string.settings_twofa_confirmButton,
                ),
            )
        }
        TextButton(onClick = callbacks.onCancelEnrolment, enabled = !state.busy) {
            Text(stringResource(R.string.common_cancel))
        }
    }
}

/**
 * The codes that get the account back when the authenticator is gone.
 *
 * They are on screen once. The hint says so before they are read, because the
 * server kept only their hashes and there is no second chance to look.
 */
@Composable
private fun RecoveryCodes(
    codes: List<String>,
    onDone: () -> Unit,
) {
    val copy = rememberSecretCopier()
    var copied by remember(codes) { mutableStateOf(false) }

    Text(
        text = stringResource(R.string.settings_twofa_recoveryHint),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.error,
    )
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            codes.forEach { code -> Text(text = code, style = MaterialTheme.typography.bodyMedium) }
        }
    }
    if (copied) {
        Text(
            text = stringResource(R.string.settings_twofa_recoveryCopied),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.primary,
        )
    }
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
        OutlinedButton(onClick = {
            copy(codes.joinToString(separator = "\n"))
            copied = true
        }, modifier = Modifier.weight(1f)) {
            Text(stringResource(R.string.settings_twofa_copyRecovery))
        }
        Button(onClick = onDone, modifier = Modifier.weight(1f)) {
            Text(stringResource(R.string.settings_twofa_finishRecovery))
        }
    }
}

@Composable
private fun DisableForm(
    state: TwoFactorUiState,
    callbacks: TwoFactorCallbacks,
) {
    CodeField(
        code = state.code,
        labelRes = R.string.settings_twofa_codeOrRecoveryPlaceholder,
        enabled = !state.busy,
        onEditCode = callbacks.onEditCode,
    )
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
        Button(
            onClick = callbacks.onDisable,
            enabled = !state.busy && state.code.isNotBlank(),
            modifier = Modifier.weight(1f),
        ) {
            Text(
                stringResource(
                    if (state.busy) R.string.settings_twofa_disabling else R.string.settings_twofa_disableConfirm,
                ),
            )
        }
        TextButton(onClick = callbacks.onCancelDisabling, enabled = !state.busy) {
            Text(stringResource(R.string.common_cancel))
        }
    }
}

@Composable
private fun CodeField(
    code: String,
    labelRes: Int,
    enabled: Boolean,
    onEditCode: (String) -> Unit,
) {
    OutlinedTextField(
        value = code,
        onValueChange = onEditCode,
        label = { Text(stringResource(labelRes)) },
        enabled = enabled,
        singleLine = true,
        // A recovery code is not numeric, so the keyboard is only hinted at
        // rather than restricted: a locked-out user typing letters must not find
        // the field refusing them.
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword),
        modifier = Modifier.fillMaxWidth(),
    )
}

/**
 * The secret as the server drew it.
 *
 * Decoded here rather than rendered on the device so the picture and the text
 * are guaranteed to be the same secret. A server that sent no image, or one this
 * device cannot decode, simply leaves the text form — which is enough to enrol.
 */
@Composable
private fun qrImage(enrolment: TotpEnrolmentDto): ImageBitmap? =
    remember(enrolment.qrPngBase64) {
        val encoded = enrolment.qrPngBase64
        if (encoded.isEmpty()) return@remember null
        try {
            val bytes = Base64.decode(encoded, Base64.DEFAULT)
            BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()
        } catch (_: IllegalArgumentException) {
            null
        }
    }

private val QR_SIZE = 200.dp

@Preview(showBackground = true)
@Composable
private fun TwoFactorScreenPreview() {
    TurboistTheme(dynamicColor = false) {
        TwoFactorContent(
            state = TwoFactorUiState(loading = false, enabled = true),
            callbacks = TwoFactorCallbacks(),
        )
    }
}
