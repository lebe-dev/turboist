import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	compilerOptions: {
		runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true)
	},
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html',
			precompress: false,
			strict: true
		}),
		// Poll for new deploys so the client can detect when its immutable
		// chunks are stale and force a full reload before navigating into them
		// (avoids the "Cannot read properties of undefined (reading 'universal')"
		// router crash when a tab outlives a deploy).
		version: {
			pollInterval: 60_000
		}
	}
};

export default config;
