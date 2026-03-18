const IOS_DISMISS_KEY = 'turboist:ios-install-dismissed';

export function shouldShowIOSInstallBanner(): boolean {
	// Already installed as PWA
	if ('standalone' in navigator && (navigator as Record<string, unknown>).standalone) return false;

	// Not iOS
	const isIOS =
		/iPad|iPhone|iPod/.test(navigator.userAgent) ||
		(navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
	if (!isIOS) return false;

	// Already dismissed
	if (localStorage.getItem(IOS_DISMISS_KEY)) return false;

	return true;
}

export function dismissIOSInstallBanner(): void {
	localStorage.setItem(IOS_DISMISS_KEY, '1');
}
