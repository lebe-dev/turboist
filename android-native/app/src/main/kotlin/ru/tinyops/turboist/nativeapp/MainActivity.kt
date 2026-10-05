package ru.tinyops.turboist.nativeapp

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.flow.MutableSharedFlow
import ru.tinyops.turboist.nativeapp.navigation.PendingTaskLinks
import ru.tinyops.turboist.nativeapp.navigation.taskLinkServerId
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddRequests
import ru.tinyops.turboist.nativeapp.quickadd.quickAddRequest
import ru.tinyops.turboist.nativeapp.shell.TurboistRoot
import javax.inject.Inject

/**
 * The single activity the app runs in. Navigation happens inside Compose, so
 * this class stays a thin host for the content tree plus the one thing Compose
 * cannot see on its own: links delivered after the activity already exists.
 */
@AndroidEntryPoint
class MainActivity : ComponentActivity() {
    // The activity is single-top, so a link that arrives while it is alive comes
    // back through onNewIntent instead of creating a second instance. Buffering
    // one keeps a link that lands before the graph is composed from being lost.
    private val newIntents = MutableSharedFlow<Intent>(replay = 1, extraBufferCapacity = 1)

    /**
     * Where a capture asked for from outside waits.
     *
     * A share or the launcher's own shortcut can land before there is anything to
     * show it — during sign-in, or on a cold start. Parking the request means
     * this class does not have to know what state the app is in.
     */
    @Inject
    lateinit var quickAddRequests: QuickAddRequests

    /**
     * Where a link from outside waits.
     *
     * The graph follows a link the moment it has one to follow, but a link can
     * arrive before there is a graph at all — on a cold start, or while nobody
     * is signed in — and parking it is what stops it being dropped on the
     * sign-in screen.
     */
    @Inject
    lateinit var pendingTaskLinks: PendingTaskLinks

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        // The launch intent is read here for both of the things it can carry. It
        // is not forwarded down the flow below: the graph reads links out of it
        // by itself, and handing it over again would open the same destination
        // twice. The parked copy is the one that survives a sign-in, and it is
        // followed as a single top entry so the two cannot stack up.
        intent?.quickAddRequest()?.let(quickAddRequests::offer)
        intent?.taskLinkServerId()?.let(pendingTaskLinks::offer)
        setContent {
            TurboistRoot(newIntents = newIntents)
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        intent.quickAddRequest()?.let(quickAddRequests::offer)
        intent.taskLinkServerId()?.let(pendingTaskLinks::offer)
        newIntents.tryEmit(intent)
    }
}
