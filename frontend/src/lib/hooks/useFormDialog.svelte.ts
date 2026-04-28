import { toast } from 'svelte-sonner';
import { describeError } from '$lib/utils/taskActions';

export function useFormDialog() {
	let submitting = $state(false);

	async function submit<T>(
		fn: () => Promise<T>,
		messages: { success: string; error: string }
	): Promise<T | undefined> {
		if (submitting) return;
		submitting = true;
		try {
			const result = await fn();
			toast.success(messages.success);
			return result;
		} catch (err) {
			toast.error(describeError(err, messages.error));
		} finally {
			submitting = false;
		}
	}

	return {
		get submitting() {
			return submitting;
		},
		submit
	};
}
