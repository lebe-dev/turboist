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
	let requestSeq = 0;

	async function refetch(): Promise<void> {
		const my = ++requestSeq;
		loading = true;
		try {
			await fetcher(() => my === requestSeq);
		} catch (err) {
			if (my !== requestSeq) return;
			if (opts?.onError) {
				opts.onError(err);
			} else {
				toast.error(describeError(err, opts?.errorMessage ?? 'Failed to load'));
			}
		} finally {
			if (my === requestSeq) loading = false;
		}
	}

	if (autoLoad) {
		onMount(refetch);
	}

	return {
		get loading() {
			return loading;
		},
		refetch
	};
}
