import { onMount } from 'svelte';
import { toast } from 'svelte-sonner';
import { describeError } from '$lib/utils/taskActions';
import { ApiError } from '$lib/api/errors';

function isNetworkError(err: unknown): boolean {
	if (err instanceof ApiError) return err.code === 'network_error';
	if (err instanceof TypeError && err.message.toLowerCase().includes('fetch')) return true;
	if (typeof navigator !== 'undefined' && !navigator.onLine) return true;
	return false;
}

export function usePageLoad(
	fetcher: (isValid: () => boolean) => Promise<void>,
	opts?: {
		errorMessage?: string;
		autoLoad?: boolean;
		initialLoading?: boolean;
		onError?: (err: unknown) => void;
		offlineFallback?: (isValid: () => boolean) => Promise<void>;
	}
) {
	const autoLoad = opts?.autoLoad !== false;
	let loading = $state(opts?.initialLoading ?? autoLoad);
	let error = $state<string | null>(null);
	let stale = $state(false);
	let refetchSeq = 0;
	let revalSeq = 0;

	async function refetch(): Promise<void> {
		const myFetch = ++refetchSeq;
		++revalSeq;
		loading = true;
		error = null;
		try {
			await fetcher(() => myFetch === refetchSeq);
			if (myFetch === refetchSeq) stale = false;
		} catch (err) {
			if (myFetch !== refetchSeq) return;
			if (opts?.offlineFallback && isNetworkError(err)) {
				try {
					await opts.offlineFallback(() => myFetch === refetchSeq);
					if (myFetch === refetchSeq) {
						stale = true;
						error = null;
					}
					return;
				} catch (fallbackErr) {
					if (myFetch !== refetchSeq) return;
					console.warn('usePageLoad: offline fallback failed', fallbackErr);
				}
			}
			const msg = describeError(err, opts?.errorMessage ?? 'Failed to load');
			error = msg;
			if (opts?.onError) {
				opts.onError(err);
			} else if (!(isNetworkError(err) && typeof navigator !== 'undefined' && !navigator.onLine)) {
				toast.error(msg);
			}
		} finally {
			if (myFetch === refetchSeq) loading = false;
		}
	}

	if (autoLoad) {
		onMount(refetch);
	}

	function cancel(): void {
		++refetchSeq;
		++revalSeq;
		loading = false;
	}

	async function revalidate(): Promise<void> {
		const my = ++revalSeq;
		try {
			await fetcher(() => my === revalSeq);
			if (my === revalSeq) stale = false;
		} catch (err) {
			if (my !== revalSeq) return;
			if (opts?.offlineFallback && isNetworkError(err)) {
				try {
					await opts.offlineFallback(() => my === revalSeq);
					if (my === revalSeq) stale = true;
				} catch (fallbackErr) {
					console.warn('usePageLoad: offline fallback failed', fallbackErr);
				}
				return;
			}
			console.warn('usePageLoad: revalidate failed', err);
		}
	}

	return {
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		get stale() {
			return stale;
		},
		refetch,
		revalidate,
		cancel
	};
}
