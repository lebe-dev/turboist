<script lang="ts">
	import XIcon from 'phosphor-svelte/lib/X';
	import ArrowUUpLeftIcon from 'phosphor-svelte/lib/ArrowUUpLeft';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { toast } from 'svelte-sonner';
	import { describeError } from '$lib/utils/taskActions';
	import { t } from '$lib/i18n';
	import type { Task } from '$lib/api/types';

	let {
		toastId,
		task,
		undo
	}: { toastId: number | string; task: Task; undo: () => Promise<void> } = $props();

	async function handleUndo(): Promise<void> {
		toast.dismiss(toastId);
		try {
			await undo();
		} catch (err) {
			toast.error(describeError(err, $t('task.toast.failedUndo')));
		}
	}

	function handleNext(): void {
		window.dispatchEvent(new CustomEvent('turboist:followUpNext', { detail: { task } }));
		toast.dismiss(toastId);
	}
</script>

<div
	class="flex w-full flex-col gap-2 rounded-md border border-border bg-popover p-3 text-popover-foreground shadow-lg"
	role="status"
>
	<div class="flex items-start justify-between gap-2">
		<div class="min-w-0 flex-1">
			<p class="text-xs font-medium">{$t('view.taskCompleted')}</p>
			<p class="mt-0.5 truncate text-xs text-muted-foreground" title={task.title}>
				{task.title}
			</p>
		</div>
		<button
			type="button"
			onclick={() => toast.dismiss(toastId)}
			aria-label={$t('view.dismiss')}
			class="rounded p-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
		>
			<XIcon class="size-3.5" />
		</button>
	</div>
	<div class="flex items-center gap-1.5">
		<button
			type="button"
			onclick={handleNext}
			class="inline-flex h-7 flex-1 items-center justify-center gap-1 rounded-md bg-primary px-2.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90"
		>
			<PlusIcon class="size-3.5" />
			{$t('view.nextTask')}
		</button>
		<button
			type="button"
			onclick={handleUndo}
			class="inline-flex h-7 items-center gap-1 rounded-md border border-border bg-background px-2.5 text-xs font-medium transition-colors hover:bg-accent hover:text-accent-foreground"
		>
			<ArrowUUpLeftIcon class="size-3.5" />
			{$t('view.undo')}
		</button>
	</div>
</div>
