import { Capacitor } from '@capacitor/core';
import type { ClientKind } from '$lib/api/types';

// Runtime platform detection. On the web build Capacitor's shim reports
// isNativePlatform() === false and getPlatform() === 'web', so the same bundle
// serves the Go-embedded web app and the native shells unchanged. Never call
// these at module top level — only inside functions, so tests that construct
// ApiClient/AuthStore directly never touch the Capacitor bridge.

export function isNativePlatform(): boolean {
	return Capacitor.isNativePlatform();
}

export function resolveClientKind(): ClientKind {
	if (!Capacitor.isNativePlatform()) return 'web';
	return Capacitor.getPlatform() === 'ios' ? 'ios' : 'android';
}
