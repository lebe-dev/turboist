<script lang="ts">
	import { createTask } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import { logger } from '$lib/stores/logger';
	import LightbulbIcon from '@lucide/svelte/icons/lightbulb';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import { toast } from 'svelte-sonner';
	import { t } from 'svelte-intl-precompile';

	let { open = $bindable(false) }: { open: boolean } = $props();

	let content = $state('');
	let description = $state('');
	let submitting = $state(false);
	let priority = $state(1);
	let today = $state(false);
	const parentTaskId = $derived(appStore.quickCapture?.parent_task_id ?? null);
	let loadError = $state<string | null>(null);

	let contentInput: HTMLInputElement | undefined = $state();
	let dialogEl: HTMLDivElement | undefined = $state();

	const priorityItems = [
		{ value: 4, label: 'P1', color: 'text-red-500', bg: 'bg-red-500/15 border-red-500/30' },
		{ value: 3, label: 'P2', color: 'text-amber-500', bg: 'bg-amber-500/15 border-amber-500/30' },
		{ value: 2, label: 'P3', color: 'text-blue-400', bg: 'bg-blue-400/15 border-blue-400/30' },
		{ value: 1, label: 'P4', color: 'text-muted-foreground', bg: 'bg-muted border-muted-foreground/30' }
	];

	$effect(() => {
		if (open) {
			content = 'turboist: ';
			description = '';
			submitting = false;
			priority = 1;
			today = false;
			loadError = parentTaskId ? null : $t('quickCapture.notConfigured');
			requestAnimationFrame(() => {
				if (contentInput) {
					contentInput.focus();
					contentInput.setSelectionRange(content.length, content.length);
				}
			});
		}
	});

	function handleSubmit() {
		if (!content.trim() || submitting || !parentTaskId) return;
		submitting = true;
		const taskContent = content.trim();
		const taskDescription = description.trim();
		const pri = priority;
		const dueDate = today ? new Date().toISOString().slice(0, 10) : undefined;

		// Optimistic: close immediately, send in background
		open = false;
		toast.success($t('quickCapture.ideaSaved'));

		createTask({
			content: taskContent,
			description: taskDescription,
			labels: [],
			priority: pri,
			parent_id: parentTaskId,
			...(dueDate ? { due_date: dueDate } : {})
		}).catch((e) => {
			logger.error('tasks', `quick capture failed: ${e}`);
			toast.error($t('quickCapture.ideaSaveFailed'));
		}).finally(() => {
			submitting = false;
		});
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
	<span class="text-[12px] underline">{$t('quickCapture.title')}</span>
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

				<!-- Toolbar -->
				<div class="flex items-center gap-1.5 px-3 pb-3">
					{#each priorityItems as p (p.value)}
						<button
							class="flex h-7 items-center gap-1 rounded-md border px-2 text-[12px] font-medium transition-colors
								{priority === p.value
									? p.bg + ' ' + p.color
									: 'border-transparent ' + p.color + ' opacity-40 hover:opacity-80 hover:border-border/50'}"
							onclick={() => { priority = p.value; }}
						>
							<FlagIcon class="h-3 w-3" />
							{p.label}
						</button>
					{/each}

					<div class="mx-1 h-4 w-px bg-border/50"></div>

					<button
						class="flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-[12px] transition-colors
							{today
								? 'border-primary/50 bg-primary/10 text-primary'
								: 'border-border/50 text-muted-foreground opacity-60 hover:opacity-100 hover:bg-accent hover:text-foreground'}"
						onclick={() => { today = !today; }}
					>
						<CalendarIcon class="h-3.5 w-3.5" />
						{$t('due.today')}
					</button>
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
