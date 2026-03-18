import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import { readFileSync } from 'fs';
import svelteIntlPrecompile from 'svelte-intl-precompile/sveltekit-plugin';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';

const pkg = JSON.parse(readFileSync('package.json', 'utf-8'));

export default defineConfig({
	define: {
		__APP_VERSION__: JSON.stringify(pkg.version)
	},
	plugins: [
		svelteIntlPrecompile('locales'),
		tailwindcss(),
		sveltekit(),
		SvelteKitPWA({
			registerType: 'prompt',
			manifest: {
				name: 'Turboist',
				short_name: 'Turboist',
				description: 'Personal task management',
				theme_color: '#e2580e',
				background_color: '#ffffff',
				display: 'standalone',
				orientation: 'portrait',
				icons: [
					{ src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png' },
					{ src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png' },
					{
						src: '/icons/icon-maskable-192.png',
						sizes: '192x192',
						type: 'image/png',
						purpose: 'maskable'
					},
					{
						src: '/icons/icon-maskable-512.png',
						sizes: '512x512',
						type: 'image/png',
						purpose: 'maskable'
					}
				]
			},
			workbox: {
				cleanupOutdatedCaches: true,
				globPatterns: ['**/*.{js,css,html,svg,png,woff2}'],
				navigateFallback: '/index.html',
				runtimeCaching: [
					{
						urlPattern: /^\/api\/config$/,
						handler: 'NetworkFirst',
						options: {
							cacheName: 'api-config',
							expiration: { maxAgeSeconds: 86400 }
						}
					}
				]
			}
		})
	],
	server: {
		allowedHosts: ['test.home']
	}
});
