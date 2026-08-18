/**
 * base64url ↔ binary helpers for the WebAuthn JSON wire format.
 *
 * Every binary field the server sends (challenge, user handle, credential ids)
 * arrives base64url-encoded, and every binary field the authenticator produces
 * has to go back the same way — the browser API itself speaks ArrayBuffer, so
 * this pair of conversions sits on both edges of a ceremony.
 */

export function base64UrlToBytes(value: string): Uint8Array {
	const padded = value.replace(/-/g, '+').replace(/_/g, '/');
	const binary = atob(padded.padEnd(padded.length + ((4 - (padded.length % 4)) % 4), '='));
	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
	return bytes;
}

export function bytesToBase64Url(buffer: ArrayBuffer): string {
	const bytes = new Uint8Array(buffer);
	let binary = '';
	for (let i = 0; i < bytes.length; i += 1) binary += String.fromCharCode(bytes[i]);
	return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}
