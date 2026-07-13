import { App } from '@capacitor/app';
import type { URLOpenListenerEvent } from '@capacitor/app';
import { isNativePlatform } from './platform';

// Native deep links. The iOS lock-screen widget (frontend/ios/App/TurboistWidget)
// opens the app with the custom URL `turboist://quick-add`; here we turn that into
// the same `turboist:quick-add` window event the top bar's "+" button dispatches,
// so the QuickAdd dialog opens.
//
// Two arrival paths are covered:
//   - warm  — the app is already running and the (app) layout is mounted, so we
//             dispatch the event straight away and its listener opens the dialog.
//   - cold  — the widget launched the app from scratch. The event would fire
//             before the (app) layout (or even login) is on screen, so instead we
//             stash a pending flag that the (app) layout consumes once it mounts
//             (surviving the auth redirect to login → today on a fresh launch).

const QUICK_ADD_HOST = 'quick-add';

let pendingQuickAdd = false;
let appLayoutReady = false;

function isQuickAddUrl(url: string): boolean {
	try {
		const parsed = new URL(url);
		if (parsed.protocol !== 'turboist:') return false;
		// `turboist://quick-add` parses host=quick-add; be lenient about a
		// `turboist:quick-add` form too (path with no authority).
		return parsed.host === QUICK_ADD_HOST || parsed.pathname.replace(/\//g, '') === QUICK_ADD_HOST;
	} catch {
		return false;
	}
}

function fireQuickAdd(): void {
	if (appLayoutReady) {
		window.dispatchEvent(new CustomEvent('turboist:quick-add'));
		return;
	}
	pendingQuickAdd = true;
}

// Called by (app)/+layout.svelte on mount/destroy so a warm deep link dispatches
// the event while a cold one waits in `pendingQuickAdd` until the layout is ready.
export function markAppLayoutReady(): void {
	appLayoutReady = true;
}

export function markAppLayoutGone(): void {
	appLayoutReady = false;
}

// Returns whether a deep link asked for QuickAdd before the layout was ready,
// clearing the flag so it fires exactly once.
export function consumePendingQuickAdd(): boolean {
	const wanted = pendingQuickAdd;
	pendingQuickAdd = false;
	return wanted;
}

export async function initDeepLinks(): Promise<void> {
	if (!isNativePlatform()) return;

	await App.addListener('appUrlOpen', (event: URLOpenListenerEvent) => {
		if (isQuickAddUrl(event.url)) fireQuickAdd();
	});

	// Cold start: the app may have just been launched by the widget's URL.
	const launch = await App.getLaunchUrl();
	if (launch?.url && isQuickAddUrl(launch.url)) fireQuickAdd();
}
