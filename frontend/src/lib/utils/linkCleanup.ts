// Mirrors LinkCleanup.kt in android-native — keep the two in step.

const LINK_PATTERN = /https?:\/\/[^\s<>"]+/gi;
// What may follow a link in prose or markdown without being part of it.
const TRAILING_PUNCTUATION = /[.,;:!?)\]}']+$/;

const TRACKING_PARAMS = new Set([
	'gclid',
	'gclsrc',
	'dclid',
	'gbraid',
	'wbraid',
	'fbclid',
	'msclkid',
	'yclid',
	'ysclid',
	'mc_cid',
	'mc_eid',
	'igshid',
	'_ga',
	'_gl'
]);

function isTracking(pair: string): boolean {
	const name = pair.split('=')[0].toLowerCase();
	return name.startsWith('utm_') || TRACKING_PARAMS.has(name);
}

function cleanLink(link: string): string {
	const hashAt = link.indexOf('#');
	const fragment = hashAt >= 0 ? link.slice(hashAt) : '';
	const rest = hashAt >= 0 ? link.slice(0, hashAt) : link;
	const queryAt = rest.indexOf('?');
	if (queryAt < 0) return link;
	const kept = rest
		.slice(queryAt + 1)
		.split('&')
		.filter((pair) => pair !== '' && !isTracking(pair));
	const base = rest.slice(0, queryAt);
	return (kept.length > 0 ? `${base}?${kept.join('&')}` : base) + fragment;
}

/** Removes advertising and analytics parameters (utm_*, gclid, fbclid, …) from every link in text. */
export function stripTrackingParams(text: string): string {
	if (!text.includes('://')) return text;
	return text.replace(LINK_PATTERN, (match) => {
		const trailing = TRAILING_PUNCTUATION.exec(match)?.[0] ?? '';
		const link = match.slice(0, match.length - trailing.length);
		return cleanLink(link) + trailing;
	});
}
