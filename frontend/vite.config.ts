import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import svelteIntlPrecompile from 'svelte-intl-precompile/sveltekit-plugin';
import pkg from './package.json' with { type: 'json' };

export default defineConfig({
	define: {
		__APP_VERSION__: JSON.stringify(pkg.version)
	},
	plugins: [svelteIntlPrecompile('locales'), tailwindcss(), sveltekit()],
	server: {
		allowedHosts: ['test.home']
	}
});
