<script lang="ts">
	import { createTask } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import LightbulbIcon from '@lucide/svelte/icons/lightbulb';
	import { toast } from 'svelte-sonner';
	import { t } from 'svelte-intl-precompile';

	let { open = $bindable(false) }: { open: boolean } = $props();

	let content = $state('');
	let description = $state('');
	let submitting = $state(false);
	const parentTaskId = $derived(appStore.quickCapture?.parent_task_id ?? null);
	let loadError = $state<string | null>(null);

	let contentInput: HTMLInputElement | undefined = $state();
	let dialogEl: HTMLDivElement | undefined = $state();

	$effect(() => {
		if (open) {
			content = 'turboist: ';
			description = '';
			submitting = false;
			loadError = parentTaskId ? null : $t('quickCapture.notConfigured');
			requestAnimationFrame(() => {
				if (contentInput) {
					contentInput.focus();
					contentInput.setSelectionRange(content.length, content.length);
				}
			});
		}
	});

	async function handleSubmit() {
		if (!content.trim() || submitting || !parentTaskId) return;
		submitting = true;
		const taskContent = content.trim();
		const taskDescription = description.trim();

		// Optimistic: close immediately, send in background
		open = false;
		toast.success('Idea saved');

		try {
			await createTask({
				content: taskContent,
				description: taskDescription,
				labels: [],
				priority: 1,
				parent_id: parentTaskId
			});
			tasksStore.refresh();
		} catch (e) {
			console.error('Failed to create quick capture task', e);
			toast.error('Failed to save idea');
		} finally {
			submitting = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			open = false;
		}
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === dialogEl) {
			open = false;
		}
	}
</script>

<button
	class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-muted-foreground/40 transition-colors hover:text-yellow-400"
	onclick={() => (open = true)}
	title="Quick capture idea (I)"
>
	<LightbulbIcon class="h-3.5 w-3.5" />
	<span class="text-[12px]">{$t('quickCapture.title')}</span>
</button>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		bind:this={dialogEl}
		class="fixed inset-0 z-50 flex items-start justify-center bg-black/60 pt-[15vh] backdrop-blur-sm"
		onkeydown={handleKeydown}
		onclick={handleBackdropClick}
	>
		<div class="mx-4 w-full max-w-lg animate-fade-in-up rounded-xl border border-border bg-popover shadow-2xl">
			{#if loadError}
				<div class="px-4 py-6 text-center text-sm text-muted-foreground">{loadError}</div>
			{:else}
				<div class="px-4 pt-4">
					<h2 class="text-sm font-medium text-muted-foreground">{$t('quickCapture.title')}</h2>
				</div>
				<!-- Content -->
				<div class="px-4 pb-2 pt-2">
					<input
						bind:this={contentInput}
						bind:value={content}
						type="text"
						placeholder={$t('quickCapture.ideaPlaceholder')}
						class="w-full bg-transparent text-lg font-medium text-foreground placeholder:text-muted-foreground/40 focus:outline-none"
						onkeydown={(e) => {
							if (e.key === 'Enter' && !e.shiftKey) {
								e.preventDefault();
								handleSubmit();
							}
						}}
					/>
					<textarea
						bind:value={description}
						placeholder={$t('quickCapture.descriptionPlaceholder')}
						rows="1"
						class="mt-1 w-full resize-none bg-transparent text-sm text-muted-foreground placeholder:text-muted-foreground/30 focus:outline-none"
						oninput={(e) => {
							const target = e.currentTarget;
							target.style.height = 'auto';
							target.style.height = target.scrollHeight + 'px';
						}}
						onkeydown={(e) => {
							if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
								e.preventDefault();
								handleSubmit();
							}
						}}
					></textarea>
				</div>

				<!-- Footer -->
				<div class="flex items-center justify-end gap-2 border-t border-border/50 px-4 py-3">
					<button
						class="rounded-lg px-4 py-1.5 text-[13px] font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
						onclick={() => (open = false)}
					>
						{$t('dialog.cancel')}
					</button>
					<button
						class="rounded-lg px-4 py-1.5 text-[13px] font-medium transition-colors
							{content.trim() && parentTaskId
								? 'bg-primary text-primary-foreground hover:bg-primary/90'
								: 'bg-muted text-muted-foreground cursor-not-allowed'}"
						disabled={!content.trim() || submitting || !parentTaskId}
						onclick={handleSubmit}
					>
						{submitting ? $t('quickCapture.saving') : $t('quickCapture.saveIdea')}
					</button>
				</div>
			{/if}
		</div>
	</div>
{/if}
