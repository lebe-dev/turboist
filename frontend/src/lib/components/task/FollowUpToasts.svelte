<script lang="ts">
	import XIcon from 'phosphor-svelte/lib/X';
	import ArrowUUpLeftIcon from 'phosphor-svelte/lib/ArrowUUpLeft';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { followUpStore, type FollowUpItem } from '$lib/stores/followUp.svelte';
	import { toast } from 'svelte-sonner';
	import { describeError } from '$lib/utils/taskActions';
	import { t } from '$lib/i18n';

	let { onNext }: { onNext: (item: FollowUpItem) => void } = $props();

	async function handleUndo(item: FollowUpItem): Promise<void> {
		followUpStore.dismiss(item.id);
		try {
			await item.undo();
		} catch (err) {
			toast.error(describeError(err, $t('task.toast.failedUndo')));
		}
	}

	function handleNext(item: FollowUpItem): void {
		followUpStore.dismiss(item.id);
		onNext(item);
	}
</script>

<div
	class="pointer-events-none fixed bottom-4 right-4 z-50 flex w-80 max-w-[calc(100vw-2rem)] flex-col-reverse gap-2"
	aria-live="polite"
>
	{#each followUpStore.items as item (item.id)}
		<div
			class="pointer-events-auto flex flex-col gap-2 rounded-md border border-border bg-popover p-3 text-popover-foreground shadow-lg"
			role="status"
		>
			<div class="flex items-start justify-between gap-2">
				<div class="min-w-0 flex-1">
					<p class="text-xs font-medium">{$t('view.taskCompleted')}</p>
					<p class="mt-0.5 truncate text-xs text-muted-foreground" title={item.task.title}>
						{item.task.title}
					</p>
				</div>
				<button
					type="button"
					onclick={() => followUpStore.dismiss(item.id)}
					aria-label={$t('view.dismiss')}
					class="rounded p-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				>
					<XIcon class="size-3.5" />
				</button>
			</div>
			<div class="flex items-center gap-1.5">
				<button
					type="button"
					onclick={() => handleNext(item)}
					class="inline-flex h-7 flex-1 items-center justify-center gap-1 rounded-md bg-primary px-2.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90"
				>
					<PlusIcon class="size-3.5" />
					{$t('view.nextTask')}
				</button>
				<button
					type="button"
					onclick={() => handleUndo(item)}
					class="inline-flex h-7 items-center gap-1 rounded-md border border-border bg-background px-2.5 text-xs font-medium transition-colors hover:bg-accent hover:text-accent-foreground"
				>
					<ArrowUUpLeftIcon class="size-3.5" />
					{$t('view.undo')}
				</button>
			</div>
		</div>
	{/each}
</div>
