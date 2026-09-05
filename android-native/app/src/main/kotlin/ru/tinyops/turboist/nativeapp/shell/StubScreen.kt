package ru.tinyops.turboist.nativeapp.shell

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/**
 * Placeholder for a destination whose real screen has not been written yet.
 *
 * It exists so the navigation graph is complete and walkable from day one: a
 * destination that cannot be opened cannot be reviewed, and a graph with holes
 * in it hides wiring mistakes until the screen that fills the hole lands.
 */
@Composable
fun StubScreen(
    @StringRes titleRes: Int,
    modifier: Modifier = Modifier,
    argument: Long? = null,
) {
    Column(
        modifier = modifier.fillMaxSize().padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Text(
            text = stringResource(titleRes),
            style = MaterialTheme.typography.headlineSmall,
            color = MaterialTheme.colorScheme.onSurface,
            textAlign = TextAlign.Center,
        )
        Text(
            text = stringResource(R.string.native_stub_body),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(top = 8.dp),
        )
        if (argument != null) {
            Text(
                text = stringResource(R.string.native_stub_argument, argument),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 16.dp),
            )
        }
    }
}

/**
 * Shown while the stored session is being resolved. Nothing is decided yet, so
 * the screen says nothing beyond "working on it" — bouncing the user to a login
 * form that a valid stored token is about to make unnecessary would be worse
 * than a moment of blank.
 */
@Composable
fun SplashScreen(modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.fillMaxSize().padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
        Text(
            text = stringResource(R.string.native_connecting),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 16.dp),
        )
    }
}

@Preview(showBackground = true)
@Composable
private fun StubScreenPreview() {
    TurboistTheme {
        StubScreen(titleRes = R.string.nav_today, argument = 42L)
    }
}

@Preview(showBackground = true)
@Composable
private fun SplashScreenPreview() {
    TurboistTheme {
        SplashScreen()
    }
}
