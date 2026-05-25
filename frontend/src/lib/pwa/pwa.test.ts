import { describe, it, expect } from 'vitest';
import { readFileSync, existsSync } from 'node:fs';
import { resolve } from 'node:path';

const buildDir = resolve(__dirname, '../../../build');

const skip = !existsSync(buildDir);

describe.skipIf(skip)('PWA build artifacts', () => {
	it('emits a service worker', () => {
		expect(existsSync(resolve(buildDir, 'sw.js'))).toBe(true);
	});

	it('emits a web manifest with required fields and icons', () => {
		const manifestPath = resolve(buildDir, 'manifest.webmanifest');
		expect(existsSync(manifestPath)).toBe(true);
		const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
		expect(manifest.name).toBe('Turboist');
		expect(manifest.start_url).toBe('/');
		expect(manifest.display).toBe('standalone');
		expect(Array.isArray(manifest.icons)).toBe(true);
		const sizes = manifest.icons.map((i: { sizes: string }) => i.sizes);
		expect(sizes).toContain('192x192');
		expect(sizes).toContain('512x512');
		const maskable = manifest.icons.filter(
			(i: { purpose?: string }) => i.purpose === 'maskable'
		);
		expect(maskable.length).toBeGreaterThan(0);
	});

	it('service worker excludes /api/* from precache and includes navigateFallback', () => {
		const sw = readFileSync(resolve(buildDir, 'sw.js'), 'utf8');
		expect(sw).toContain('/api/');
		expect(sw.toLowerCase()).toContain('navigationroute');
	});
});
