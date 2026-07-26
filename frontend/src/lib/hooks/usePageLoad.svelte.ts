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
		// Monotonic count of local mutations applied to the data this loader owns —
		// pass `() => list.epoch` from `useListMutator`. A background revalidation
		// snapshots it and discards its response if it advanced while in flight; see
		// `revalidate` for why.
		epoch?: () => number;
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

	async function refetch(): Promise<void> {
		const myFetch = ++refetchSeq;
		++revalSeq; // cancel any in-flight revalidation
		loading = true;
		error = null;
		try {
			// No epoch guard here on purpose: refetch() is user intent (context switch,
			// route change) and its result must land even if a mutation raced it —
			// otherwise the view would keep showing the previous filter's data.
			await fetcher(() => myFetch === refetchSeq);
		} catch (err) {
			if (myFetch !== refetchSeq) return;
			const msg = describeError(err, opts?.errorMessage ?? 'Failed to load');
			error = msg;
			if (opts?.onError) {
				opts.onError(err);
			} else {
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

	// revalidate performs a background fetch without touching the `loading`
	// flag. Used by SSE-driven invalidation so views update in place instead of
	// flashing a spinner. Errors are silently dropped — the user did not
	// initiate this refresh, so a toast would be confusing; the next
	// user-triggered refetch will surface real failures.
	//
	// A revalidation also yields to any local mutation that lands while it is in
	// flight. Its payload is a snapshot of server truth from BEFORE that mutation,
	// so applying it would resurrect the task the user just completed: the classic
	// symptom is opening a long-idle tab (the throttled SSE reconnect timer fires on
	// wake and invalidates every scope), clicking a checkbox, watching the list
	// re-render with the task still there, and having to click a second time.
	async function revalidate(): Promise<void> {
		return runRevalidation(false);
	}

	async function runRevalidation(isRetry: boolean): Promise<void> {
		const my = ++revalSeq;
		const epochAtStart = opts?.epoch?.();
		// Set when the guard rejected specifically because of a local write, as
		// opposed to a newer load superseding this one.
		let supersededByWrite = false;
		const isValid = (): boolean => {
			if (my !== revalSeq) return false;
			if (epochAtStart !== undefined && opts?.epoch?.() !== epochAtStart) {
				supersededByWrite = true;
				return false;
			}
			return true;
		};
		try {
			await fetcher(isValid);
		} catch (err) {
			if (my !== revalSeq) return;
			console.warn('usePageLoad: revalidate failed', err);
			return;
		}
		// The snapshot we threw away may also have carried a genuinely remote change,
		// and this tab's own mutation raises no `tasks` invalidation to fetch it later
		// (its SSE echo is suppressed). So fetch once more, now that the local write
		// is in. Once only: if the user keeps mutating we stop and let the next
		// invalidation catch up, rather than chasing a moving target.
		if (supersededByWrite && !isRetry && my === revalSeq) await runRevalidation(true);
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
