import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import { readFileSync } from 'fs';
import svelteIntlPrecompile from 'svelte-intl-precompile/sveltekit-plugin';

const pkg = JSON.parse(readFileSync('package.json', 'utf-8'));

export default defineConfig({
	define: {
		__APP_VERSION__: JSON.stringify(pkg.version)
	},
	plugins: [svelteIntlPrecompile('locales'), tailwindcss(), sveltekit()],
	server: {
		allowedHosts: ['test.home']
	}
});
