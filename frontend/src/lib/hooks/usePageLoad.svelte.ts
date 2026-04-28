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
	let requestSeq = 0;

	async function refetch(): Promise<void> {
		const my = ++requestSeq;
		loading = true;
		error = null;
		try {
			await fetcher(() => my === requestSeq);
		} catch (err) {
			if (my !== requestSeq) return;
			const msg = describeError(err, opts?.errorMessage ?? 'Failed to load');
			error = msg;
			if (opts?.onError) {
				opts.onError(err);
			} else {
				toast.error(msg);
			}
		} finally {
			if (my === requestSeq) loading = false;
		}
	}

	if (autoLoad) {
		onMount(refetch);
	}

	function cancel(): void {
		requestSeq++;
		loading = false;
	}

	return {
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		refetch,
		cancel
	};
}
