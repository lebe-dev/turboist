export async function isOnline(): Promise<boolean> {
	if (!navigator.onLine) return false;

	try {
		const res = await fetch('/api/health', {
			method: 'HEAD',
			cache: 'no-store',
			signal: AbortSignal.timeout(3000)
		});
		return res.ok;
	} catch {
		return false;
	}
}
