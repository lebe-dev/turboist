import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// F3.4 i18n parity: every new "updated remotely" open-card key must exist in
// both en.json and ru.json (English is the source of truth and fallback).
//
// The locale JSON is read from disk rather than imported, because the
// svelte-intl-precompile Vite plugin rewrites `locales/*.json` into compiled
// message modules at import time.

type Dict = Record<string, unknown>;

function loadLocale(name: string): Dict {
	// Resolve from the Vitest working directory (frontend/) at runtime so the
	// bundler cannot statically rewrite this into a precompiled message module.
	const path = join(process.cwd(), 'locales', `${name}.json`);
	return JSON.parse(readFileSync(path, 'utf8')) as Dict;
}

function get(obj: Dict, path: string): unknown {
	return path.split('.').reduce<unknown>((acc, key) => {
		if (acc && typeof acc === 'object') return (acc as Dict)[key];
		return undefined;
	}, obj);
}

const en = loadLocale('en');
const ru = loadLocale('ru');

const REMOTE_EDIT_KEYS = [
	'federation.remoteEdit.title',
	'federation.remoteEdit.body',
	'federation.remoteEdit.reload',
	'federation.remoteEdit.keepEditing'
];

describe('F3.4 i18n parity — federation.remoteEdit', () => {
	it.each(REMOTE_EDIT_KEYS)('%s exists in en.json', (key) => {
		expect(typeof get(en, key)).toBe('string');
	});

	it.each(REMOTE_EDIT_KEYS)('%s exists in ru.json', (key) => {
		expect(typeof get(ru, key)).toBe('string');
	});
});
