import js from '@eslint/js';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import ts from 'typescript-eslint';
import svelteConfig from './svelte.config.js';

export default ts.config(
	{
		ignores: [
			'src/lib/components/ui/**',
			'build/**',
			'.svelte-kit/**',
			'node_modules/**',
			'static/**',
			// Native Capacitor projects (frontend/ios, frontend/android): contain
			// copied web build output, Gradle/Xcode build dirs, and vendored
			// Cordova plugin JS — not source we own or want linted.
			'ios/**',
			'android/**',
			'coverage/**'
		]
	},
	js.configs.recommended,
	...ts.configs.recommended,
	...svelte.configs.recommended,
	{
		languageOptions: {
			globals: {
				...globals.browser,
				...globals.node,
				__APP_VERSION__: 'readonly'
			}
		}
	},
	{
		files: ['**/*.svelte', '**/*.svelte.ts', '**/*.svelte.js'],
		languageOptions: {
			parserOptions: {
				projectService: true,
				extraFileExtensions: ['.svelte'],
				parser: ts.parser,
				svelteConfig
			}
		}
	},
	{
		rules: {
			'@typescript-eslint/no-unused-vars': [
				'warn',
				{ argsIgnorePattern: '^_', varsIgnorePattern: '^_' }
			]
		}
	}
);
