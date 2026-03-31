<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { portal } from '$lib/utils/portal';
	import { t } from 'svelte-intl-precompile';
	import { tick } from 'svelte';

	let {
		open = $bindable(false),
		task,
		onConfirm
	}: {
		open: boolean;
		task: Task;
		onConfirm: (taskNames: string[]) => void;
	} = $props();

	let textareaValue = $state('');
	let textareaEl: HTMLTextAreaElement | undefined = $state();

	const taskNames = $derived(
		textareaValue
			.split('\n')
			.map((s) => s.trim())
			.filter((s) => s.length > 0)
	);

	const canSubmit = $derived(taskNames.length > 0);

	$effect(() => {
		if (open) {
			textareaValue = '';
			tick().then(() => textareaEl?.focus());
		}
	});

	function handleConfirm() {
		if (!canSubmit) return;
		onConfirm(taskNames);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			open = false;
		}
		if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && canSubmit) {
			e.preventDefault();
			handleConfirm();
		}
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<div
		use:portal
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
		onclick={() => { open = false; }}
		onkeydown={handleKeydown}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-md rounded-lg border border-border bg-background p-6 shadow-xl"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="text-lg font-semibold text-foreground">{$t('task.decomposeTitle')}</h3>
			<p class="mt-1 truncate text-sm text-muted-foreground">{task.content}</p>

			<textarea
				bind:this={textareaEl}
				bind:value={textareaValue}
				placeholder={$t('task.decomposeDescription')}
				class="mt-4 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
				rows="5"
				onkeydown={handleKeydown}
			></textarea>

			{#if taskNames.length > 0}
				<p class="mt-1 text-xs text-muted-foreground">
					{taskNames.length} {taskNames.length === 1 ? 'task' : 'tasks'}
				</p>
			{/if}

			<div class="mt-4 flex justify-end gap-2">
				<button
					class="rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
					onclick={() => { open = false; }}
				>
					{$t('dialog.cancel')}
				</button>
				<button
					class="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
					disabled={!canSubmit}
					onclick={handleConfirm}
				>
					{$t('task.decomposeButton')}
				</button>
			</div>
		</div>
	</div>
{/if}
