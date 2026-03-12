import { goto } from '$app/navigation';
import { login as apiLogin, logout as apiLogout, me } from '$lib/api/client';

type AuthState = 'unknown' | 'authenticated' | 'unauthenticated';

function createAuthStore() {
	let state = $state<AuthState>('unknown');

	async function check(): Promise<void> {
		try {
			await me();
			state = 'authenticated';
		} catch {
			state = 'unauthenticated';
		}
	}

	async function login(password: string): Promise<void> {
		await apiLogin(password);
		state = 'authenticated';
		goto('/');
	}

	async function logout(): Promise<void> {
		await apiLogout();
		state = 'unauthenticated';
		goto('/login');
	}

	function requireAuth(): void {
		if (state === 'unauthenticated') {
			goto('/login');
		}
	}

	return {
		get state() {
			return state;
		},
		get isAuthenticated() {
			return state === 'authenticated';
		},
		check,
		login,
		logout,
		requireAuth
	};
}

export const auth = createAuthStore();
