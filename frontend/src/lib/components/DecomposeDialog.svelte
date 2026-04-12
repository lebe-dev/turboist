<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { portal } from '$lib/utils/portal';
	import { t } from 'svelte-intl-precompile';
	import { tick } from 'svelte';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import FlagIcon from '@lucide/svelte/icons/flag';

	let {
		open = $bindable(false),
		task,
		onConfirm
	}: {
		open: boolean;
		task: Task;
		onConfirm: (taskNames: string[], priority?: number, dueDate?: string) => void;
	} = $props();

	let textareaValue = $state('');
	let textareaEl: HTMLTextAreaElement | undefined = $state();
	let dueDate = $state('');
	let priority = $state(0);
	let showPriorityPicker = $state(false);

	const priorityItems = [
		{ value: 4, label: 'P1', color: 'text-red-500' },
		{ value: 3, label: 'P2', color: 'text-amber-500' },
		{ value: 2, label: 'P3', color: 'text-blue-400' },
		{ value: 1, label: 'P4', color: 'text-muted-foreground' }
	];

	function localISODate(offsetDays = 0): string {
		const d = new Date();
		d.setDate(d.getDate() + offsetDays);
		return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	}

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
			dueDate = '';
			priority = 0;
			showPriorityPicker = false;
			tick().then(() => textareaEl?.focus());
		}
	});

	function handleConfirm() {
		if (!canSubmit) return;
		onConfirm(
			taskNames,
			priority > 0 ? priority : undefined,
			dueDate || undefined
		);
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
			onclick={(e) => { e.stopPropagation(); showPriorityPicker = false; }}
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

			<div class="mt-3 flex items-center gap-1">
				<button
					class="flex items-center gap-1 rounded-md px-2 py-1 text-[12px] transition-colors
						{dueDate === localISODate(0) ? 'bg-accent text-foreground' : 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
					onclick={() => { dueDate = dueDate === localISODate(0) ? '' : localISODate(0); }}
				>
					<CalendarIcon class="h-3 w-3" />
					{$t('due.today')}
				</button>
				<button
					class="flex items-center gap-1 rounded-md px-2 py-1 text-[12px] transition-colors
						{dueDate === localISODate(1) ? 'bg-accent text-foreground' : 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
					onclick={() => { dueDate = dueDate === localISODate(1) ? '' : localISODate(1); }}
				>
					<CalendarIcon class="h-3 w-3" />
					{$t('due.tomorrow')}
				</button>
				<div class="relative">
					<button
						class="flex items-center gap-1 rounded-md px-2 py-1 text-[12px] transition-colors
							{priority > 0 ? (priorityItems.find(p => p.value === priority)?.color ?? 'text-muted-foreground') : 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
						onclick={(e) => { e.stopPropagation(); showPriorityPicker = !showPriorityPicker; }}
					>
						<FlagIcon class="h-3 w-3" />
						{priority > 0 ? (priorityItems.find(p => p.value === priority)?.label ?? '') : $t('task.priority')}
					</button>
					{#if showPriorityPicker}
						<div class="absolute bottom-full left-0 z-10 mb-1 w-32 rounded-lg border border-border bg-popover shadow-xl">
							<div class="px-1 py-1">
								{#each priorityItems as p (p.value)}
									<button
										class="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-[12px] transition-colors hover:bg-accent
											{priority === p.value ? 'bg-accent' : ''}"
										onclick={() => { priority = priority === p.value ? 0 : p.value; showPriorityPicker = false; }}
									>
										<FlagIcon class="h-3 w-3 {p.color}" />
										<span class={p.color}>{p.label}</span>
									</button>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			</div>

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
