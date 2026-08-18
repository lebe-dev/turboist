import { Capacitor } from '@capacitor/core';
import type {
	PasskeyPublicKeyCredentialCreationOptionsJSON,
	PasskeyPublicKeyCredentialRequestOptionsJSON
} from '@capgo/capacitor-passkey';
import { base64UrlToBytes, bytesToBase64Url } from './encoding';

/**
 * WebAuthn (passkey) ceremonies, in the one JSON shape the backend speaks.
 *
 * The server hands out `PublicKeyCredential*OptionsJSON` (binary fields
 * base64url-encoded) and expects the authenticator's answer back in the same
 * form. Two runtimes have to produce it:
 *
 *  - web: `navigator.credentials`, which speaks ArrayBuffers, so the options are
 *    decoded on the way in and the response re-encoded on the way out;
 *  - native (Capacitor): the platform passkey APIs via `@capgo/capacitor-passkey`,
 *    which already take and return the JSON form. The iOS/Android WebView cannot
 *    run a browser ceremony for a remote relying party — the app's origin is
 *    `capacitor://localhost` / `https://localhost`, not the server's — so the
 *    native bridge is the only path there, and it needs the association files
 *    (`apple-app-site-association`, `assetlinks.json`) served by the same domain.
 *    The plugin is imported lazily so the web bundle never pulls it in.
 */

export interface PasskeyCreationOptions {
	publicKey: Record<string, unknown>;
}

export interface PasskeyRequestOptions {
	publicKey: Record<string, unknown>;
}

/** The credential JSON posted back to the server. */
export type PasskeyCredentialJSON = Record<string, unknown>;

/**
 * Whether a passkey ceremony can run at all here. Web needs the WebAuthn API
 * (absent in insecure contexts and older browsers); native always routes through
 * the plugin, whose own support probe runs when a ceremony starts.
 */
export function isPasskeySupported(): boolean {
	if (Capacitor.isNativePlatform()) return true;
	return (
		typeof window !== 'undefined' &&
		typeof window.PublicKeyCredential !== 'undefined' &&
		typeof navigator !== 'undefined' &&
		!!navigator.credentials
	);
}

export async function createPasskeyCredential(
	options: PasskeyCreationOptions
): Promise<PasskeyCredentialJSON> {
	if (Capacitor.isNativePlatform()) {
		const { CapacitorPasskey } = await import('@capgo/capacitor-passkey');
		const created = await CapacitorPasskey.createCredential({
			origin: await nativeOrigin(),
			publicKey: options.publicKey as unknown as PasskeyPublicKeyCredentialCreationOptionsJSON
		});
		return created as unknown as PasskeyCredentialJSON;
	}

	const credential = (await navigator.credentials.create({
		publicKey: toCreationOptions(options.publicKey)
	})) as PublicKeyCredential | null;
	if (!credential) throw new Error('passkey creation returned no credential');
	return registrationToJSON(credential);
}

export async function getPasskeyAssertion(
	options: PasskeyRequestOptions
): Promise<PasskeyCredentialJSON> {
	if (Capacitor.isNativePlatform()) {
		const { CapacitorPasskey } = await import('@capgo/capacitor-passkey');
		const asserted = await CapacitorPasskey.getCredential({
			// `mediation` is load-bearing, not a hint: the plugin tells an
			// assertion from a registration by whether this key is PRESENT, and
			// without it treats the call as a create — reading `publicKey.rp.id`,
			// which a login's options do not have (they carry `rpId`), and failing
			// with "Cannot read properties of undefined". Never pass it to
			// createCredential for the mirror-image reason.
			mediation: 'optional',
			origin: await nativeOrigin(),
			publicKey: options.publicKey as unknown as PasskeyPublicKeyCredentialRequestOptionsJSON
		});
		return asserted as unknown as PasskeyCredentialJSON;
	}

	const credential = (await navigator.credentials.get({
		publicKey: toRequestOptions(options.publicKey)
	})) as PublicKeyCredential | null;
	if (!credential) throw new Error('passkey assertion returned no credential');
	return assertionToJSON(credential);
}

/**
 * True when the ceremony was dismissed rather than failed — the user closed the
 * platform sheet or let it time out. Worth telling apart because it must not
 * surface as an error banner.
 */
export function isPasskeyCancellation(err: unknown): boolean {
	return (
		err instanceof DOMException && (err.name === 'NotAllowedError' || err.name === 'AbortError')
	);
}

/**
 * The HTTPS origin the native ceremony should claim, read from
 * `plugins.CapacitorPasskey.origin` in `capacitor.config.ts`.
 *
 * Passing it explicitly matters on iOS 17.4+, which encodes the origin into
 * `clientDataJSON`: left to itself the plugin falls back to `location.origin`,
 * which inside the WebView is `capacitor://localhost` / `https://localhost` —
 * an origin the server rightly refuses. (Android reports
 * `android:apk-key-hash:…` regardless, so this is a no-op there.)
 */
async function nativeOrigin(): Promise<string | undefined> {
	const { CapacitorPasskey } = await import('@capgo/capacitor-passkey');
	try {
		return (await CapacitorPasskey.getConfiguration()).origin;
	} catch {
		// An unconfigured build still works on Android; let the plugin fall back.
		return undefined;
	}
}

function toCreationOptions(json: Record<string, unknown>): PublicKeyCredentialCreationOptions {
	const source = json as {
		challenge: string;
		user: { id: string; name: string; displayName: string };
		excludeCredentials?: { id: string; type: string; transports?: string[] }[];
	};
	return {
		...(json as unknown as PublicKeyCredentialCreationOptions),
		challenge: bufferOf(source.challenge),
		user: { ...source.user, id: bufferOf(source.user.id) },
		excludeCredentials: (source.excludeCredentials ?? []).map((descriptor) => ({
			...descriptor,
			id: bufferOf(descriptor.id),
			type: 'public-key' as const,
			transports: descriptor.transports as AuthenticatorTransport[] | undefined
		}))
	};
}

function toRequestOptions(json: Record<string, unknown>): PublicKeyCredentialRequestOptions {
	const source = json as {
		challenge: string;
		allowCredentials?: { id: string; type: string; transports?: string[] }[];
	};
	return {
		...(json as unknown as PublicKeyCredentialRequestOptions),
		challenge: bufferOf(source.challenge),
		allowCredentials: (source.allowCredentials ?? []).map((descriptor) => ({
			...descriptor,
			id: bufferOf(descriptor.id),
			type: 'public-key' as const,
			transports: descriptor.transports as AuthenticatorTransport[] | undefined
		}))
	};
}

function registrationToJSON(credential: PublicKeyCredential): PasskeyCredentialJSON {
	const response = credential.response as AuthenticatorAttestationResponse;
	return {
		id: credential.id,
		rawId: bytesToBase64Url(credential.rawId),
		type: credential.type,
		authenticatorAttachment: credential.authenticatorAttachment ?? undefined,
		clientExtensionResults: credential.getClientExtensionResults(),
		response: {
			clientDataJSON: bytesToBase64Url(response.clientDataJSON),
			attestationObject: bytesToBase64Url(response.attestationObject),
			// Transports let the server hint the right authenticator next time;
			// absent on older browsers, hence the guard.
			transports:
				typeof response.getTransports === 'function' ? response.getTransports() : undefined
		}
	};
}

function assertionToJSON(credential: PublicKeyCredential): PasskeyCredentialJSON {
	const response = credential.response as AuthenticatorAssertionResponse;
	return {
		id: credential.id,
		rawId: bytesToBase64Url(credential.rawId),
		type: credential.type,
		authenticatorAttachment: credential.authenticatorAttachment ?? undefined,
		clientExtensionResults: credential.getClientExtensionResults(),
		response: {
			clientDataJSON: bytesToBase64Url(response.clientDataJSON),
			authenticatorData: bytesToBase64Url(response.authenticatorData),
			signature: bytesToBase64Url(response.signature),
			// The user handle is what makes the login usernameless: it tells the
			// server which account the authenticator just proved ownership of.
			userHandle: response.userHandle ? bytesToBase64Url(response.userHandle) : undefined
		}
	};
}

function bufferOf(value: string): ArrayBuffer {
	const bytes = base64UrlToBytes(value);
	return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}
