import { useRegisterSW } from 'virtual:pwa-register/svelte';

export const pwaUpdate = useRegisterSW({
	onRegisteredSW(swUrl, registration) {
		if (!registration) return;
		// Check for updates every 60 seconds
		setInterval(async () => {
			if (registration.installing || !navigator) return;
			if ('connection' in navigator && !navigator.onLine) return;
			await registration.update();
		}, 60_000);
	}
});
