import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import path from 'path';

export default defineConfig({
	plugins: [svelte({ hot: false })],
	resolve: {
		conditions: ['browser'],
		alias: {
			$lib: path.resolve('./src/lib'),
			'$app': path.resolve('./src/lib/__mocks__/$app')
		}
	},
	test: {
		include: ['src/**/*.test.ts'],
		environment: 'jsdom',
		setupFiles: ['./src/setup-test.ts']
	}
});
