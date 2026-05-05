import { sveltekit } from '@sveltejs/kit/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [sveltekit(), svelteTesting()],
	test: {
		environment: 'jsdom',
		globals: true,
		setupFiles: ['./vitest.setup.ts'],
		passWithNoTests: true,
		include: ['src/**/*.{test,spec}.{ts,js}'],
		exclude: ['src/lib/components/ui/**', 'node_modules/**', 'build/**', '.svelte-kit/**']
	}
});
