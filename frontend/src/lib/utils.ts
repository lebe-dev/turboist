import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

/**
 * Increment trailing `(N)` in a task title for duplicates.
 * "Buy milk (3)" → "Buy milk (4)", "Buy milk" → "Buy milk" (unchanged).
 */
export function incrementDuplicateTitle(content: string): string {
	return content.replace(/\((\d+)\)\s*$/, (_, n) => `(${Number(n) + 1})`);
}

/**
 * Strip leading prefix from a task title when copying.
 * Removes patterns like "Something: text", "Something - text", "Something – text".
 */
export function stripTaskPrefix(content: string): string {
	const stripped = content.replace(/^.+?(?::\s|\s[-–]\s)/, '');
	return stripped.trim() || content;
}

/**
 * Strip markdown link syntax, keeping only the link text.
 * "[Magic Link Pitfalls](https://example.com)" → "Magic Link Pitfalls"
 */
export function stripMarkdownLinks(text: string): string {
	return text.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1');
}

/**
 * Tracking query-params to strip from pasted URLs.
 */
const TRACKING_PARAMS = new Set([
	// Google / UTM
	'utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content',
	'utm_id', 'utm_source_platform', 'utm_creative_format', 'utm_marketing_tactic',
	// Google Ads / Click IDs
	'gclid', 'gclsrc', 'dclid', 'gbraid', 'wbraid',
	// Facebook / Meta
	'fbclid', 'fb_action_ids', 'fb_action_types', 'fb_ref', 'fb_source',
	// Microsoft
	'msclkid',
	// HubSpot
	'_hsenc', '_hsmi',
	// Mailchimp
	'mc_cid', 'mc_eid',
	// Twitter / X
	'twclid',
	// Yahoo
	'yclid',
	// Instagram
	'igshid',
	// YouTube
	'si', 'feature',
]);

function cleanUrl(raw: string): string {
	try {
		const u = new URL(raw);
		let changed = false;
		for (const key of [...u.searchParams.keys()]) {
			if (TRACKING_PARAMS.has(key)) {
				u.searchParams.delete(key);
				changed = true;
			}
		}
		if (!changed) return raw;
		let result = u.toString();
		// Remove trailing '?' if all params were stripped
		if (result.endsWith('?')) result = result.slice(0, -1);
		return result;
	} catch {
		return raw;
	}
}

/**
 * Clean tracking parameters (utm_*, gclid, fbclid, etc.) from all URLs in text.
 */
export function cleanTrackingParams(text: string): string {
	return text.replace(/https?:\/\/[^\s)\]>]+/g, (match) => cleanUrl(match));
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChild<T> = T extends { child?: any } ? Omit<T, "child"> : T;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChildren<T> = T extends { children?: any } ? Omit<T, "children"> : T;
export type WithoutChildrenOrChild<T> = WithoutChildren<WithoutChild<T>>;
export type WithElementRef<T, U extends HTMLElement = HTMLElement> = T & { ref?: U | null };
