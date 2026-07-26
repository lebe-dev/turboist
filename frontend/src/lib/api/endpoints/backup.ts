import { ApiError, isApiErrorEnvelope } from '../errors';
import { getApiClient } from '../client';
import { getAuthStore } from '$lib/auth/store.svelte';

// Backup endpoints bypass the typed ApiClient since they exchange binary
// payloads (gzipped JSON on download, file Blob on upload) rather than JSON
// envelopes. Auth is still applied via the same access token.

function authHeader(): Record<string, string> {
	const token = getAuthStore().accessToken;
	if (!token) {
		throw new ApiError('auth_invalid', 'not authenticated', 401);
	}
	return { Authorization: `Bearer ${token}` };
}

async function asApiError(response: Response): Promise<ApiError> {
	try {
		const data = await response.clone().json();
		if (isApiErrorEnvelope(data)) {
			return new ApiError(data.error.code, data.error.message, response.status, data.error.details);
		}
	} catch {
		// fall through
	}
	return new ApiError('unknown_error', `HTTP ${response.status}`, response.status);
}

export const backup = {
	async download(includeSettings: boolean): Promise<{ blob: Blob; filename: string }> {
		const url = `${getApiClient().baseUrl}/api/v1/backup${includeSettings ? '?settings=1' : ''}`;
		const response = await fetch(url, { headers: authHeader() });
		if (!response.ok) {
			throw await asApiError(response);
		}
		const blob = await response.blob();
		const filename = filenameFromDisposition(response.headers.get('Content-Disposition')) ?? defaultFilename();
		return { blob, filename };
	},

	async restore(file: File): Promise<void> {
		const response = await fetch(`${getApiClient().baseUrl}/api/v1/restore`, {
			method: 'POST',
			headers: { ...authHeader(), 'Content-Type': 'application/octet-stream' },
			body: file
		});
		if (response.status === 204) return;
		throw await asApiError(response);
	}
};

function filenameFromDisposition(value: string | null): string | null {
	if (!value) return null;
	const match = /filename="([^"]+)"/.exec(value);
	return match?.[1] ?? null;
}

function defaultFilename(): string {
	const d = new Date();
	const yyyy = d.getUTCFullYear();
	const mm = String(d.getUTCMonth() + 1).padStart(2, '0');
	const dd = String(d.getUTCDate()).padStart(2, '0');
	return `turboist-backup-${yyyy}${mm}${dd}.json`;
}
