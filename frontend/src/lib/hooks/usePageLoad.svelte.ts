import { onMount } from 'svelte';
import { toast } from 'svelte-sonner';
import { describeError } from '$lib/utils/taskActions';

export function usePageLoad(
	fetcher: (isValid: () => boolean) => Promise<void>,
	opts?: {
		errorMessage?: string;
		autoLoad?: boolean;
		initialLoading?: boolean;
		onError?: (err: unknown) => void;
	}
) {
	const autoLoad = opts?.autoLoad !== false;
	let loading = $state(opts?.initialLoading ?? autoLoad);
	let error = $state<string | null>(null);
	// Two independent counters so revalidate() can never prevent refetch() from
	// resetting the loading flag. refetchSeq guards loading; revalSeq guards
	// background revalidations. refetch() bumps both to cancel stale revals.
	let refetchSeq = 0;
	let revalSeq = 0;

	// refetch runs the user-initiated load. It resolves to `true` on success and
	// `false` when the fetch failed (after running onError / toasting). The result
	// lets a caller that DID initiate the fetch (e.g. the F3.4 explicit Reload)
	// react to a failure — unlike the error-swallowing revalidate() background
	// path. A superseded/cancelled refetch also resolves to `false`.
	async function refetch(): Promise<boolean> {
		const myFetch = ++refetchSeq;
		++revalSeq; // cancel any in-flight revalidation
		loading = true;
		error = null;
		try {
			await fetcher(() => myFetch === refetchSeq);
			return myFetch === refetchSeq;
		} catch (err) {
			if (myFetch !== refetchSeq) return false;
			const msg = describeError(err, opts?.errorMessage ?? 'Failed to load');
			error = msg;
			if (opts?.onError) {
				opts.onError(err);
			} else {
				toast.error(msg);
			}
			return false;
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

	// revalidate performs a background fetch without touching the `loading`
	// flag. Used by SSE-driven invalidation so views update in place instead of
	// flashing a spinner. Errors are silently dropped — the user did not
	// initiate this refresh, so a toast would be confusing; the next
	// user-triggered refetch will surface real failures.
	async function revalidate(): Promise<void> {
		const my = ++revalSeq;
		try {
			await fetcher(() => my === revalSeq);
		} catch (err) {
			if (my !== revalSeq) return;
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
		refetch,
		revalidate,
		cancel
	};
}
