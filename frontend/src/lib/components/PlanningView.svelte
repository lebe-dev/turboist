<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { planningStore } from '$lib/stores/planning.svelte';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import BacklogPanel from './BacklogPanel.svelte';
	import WeeklyPlanningPanel from './WeeklyPlanningPanel.svelte';
	import WeeklyProgress from './WeeklyProgress.svelte';
	import PlayIcon from '@lucide/svelte/icons/play';
	import XIcon from '@lucide/svelte/icons/x';
	import { t } from 'svelte-intl-precompile';
	import { toast } from 'svelte-sonner';

	let showStartConfirm = $state(false);
	let starting = $state(false);

	async function handleStart() {
		starting = true;
		try {
			await planningStore.startWeek();
			showStartConfirm = false;
		} catch (e) {
			if (e instanceof Error && e.message === 'offline:not-queueable') {
				toast.error($t('pwa.requiresNetwork'));
			}
		} finally {
			starting = false;
		}
	}

	const progressPercent = $derived(
		planningStore.meta.weekly_limit > 0
			? Math.min(100, Math.round((planningStore.meta.weekly_count / planningStore.meta.weekly_limit) * 100))
			: 0
	);
	const progressIsOver = $derived(planningStore.meta.weekly_count >= planningStore.meta.weekly_limit);
	const progressIsWarning = $derived(progressPercent >= 80 && !progressIsOver);

	onMount(() => {
		tasksStore.stop();
		planningStore.enter();
	});

	onDestroy(() => {
		planningStore.exit();
		tasksStore.start();
	});

	function handleExit() {
		planningStore.exit();
		tasksStore.start();
	}
</script>

<div class="flex h-full flex-col">
	<!-- Header -->
	<header class="flex h-12 shrink-0 items-center gap-3 border-b border-border/50 px-4 md:px-6">
		<h1 class="text-sm font-semibold tracking-wide text-foreground">{$t('planning.title')}</h1>

		<!-- Compact progress in header -->
		<div class="hidden items-center gap-2 md:flex">
			<div class="h-1.5 w-24 overflow-hidden rounded-full bg-muted">
				<div
					class="h-full rounded-full transition-all duration-500 ease-out
						{progressIsOver ? 'bg-destructive' : progressIsWarning ? 'bg-yellow-500' : 'bg-primary'}"
					style="width: {progressPercent}%"
				></div>
			</div>
			<span class="tabular-nums text-xs font-medium text-muted-foreground">
				{planningStore.meta.weekly_count}/{planningStore.meta.weekly_limit}
			</span>
		</div>

		<div class="ml-auto flex items-center gap-1">
			<button
				class="flex h-8 items-center gap-1.5 rounded-md px-2.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				onclick={() => (showStartConfirm = true)}
				title={$t('planning.start')}
			>
				<PlayIcon class="h-3.5 w-3.5" />
				<span class="hidden md:inline">{$t('planning.start')}</span>
			</button>
			<button
				class="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				onclick={handleExit}
				aria-label={$t('planning.exit')}
				title={$t('planning.exit')}
			>
				<XIcon class="h-4 w-4" />
			</button>
		</div>
	</header>

	{#if planningStore.loading}
		<div class="flex flex-1 items-center justify-center">
			<div class="h-5 w-5 animate-spin rounded-full border-2 border-primary border-t-transparent"></div>
		</div>
	{:else}
		<!-- Mobile tabs -->
		<div class="flex shrink-0 border-b border-border/50 md:hidden">
			<button
				class="flex-1 px-4 py-2.5 text-center text-sm font-medium transition-colors
					{planningStore.mobileTab === 'backlog'
					? 'border-b-2 border-primary text-foreground'
					: 'text-muted-foreground hover:text-foreground'}"
				onclick={() => (planningStore.mobileTab = 'backlog')}
			>
				{$t('planning.backlog')}
			</button>
			<button
				class="flex-1 px-4 py-2.5 text-center text-sm font-medium transition-colors
					{planningStore.mobileTab === 'weekly'
					? 'border-b-2 border-primary text-foreground'
					: 'text-muted-foreground hover:text-foreground'}"
				onclick={() => (planningStore.mobileTab = 'weekly')}
			>
				{$t('planning.thisWeek')}
				<span class="ml-1 rounded-full bg-muted px-1.5 py-0.5 text-[11px] tabular-nums">
					{planningStore.meta.weekly_count}/{planningStore.meta.weekly_limit}
				</span>
			</button>
		</div>

		<!-- Desktop: split view -->
		<div class="hidden flex-1 overflow-hidden md:flex">
			<div class="flex-1 overflow-hidden border-r border-border/50">
				<BacklogPanel />
			</div>
			<div class="flex-1 overflow-hidden">
				<WeeklyPlanningPanel />
			</div>
		</div>

		<!-- Mobile: tabbed view -->
		<div class="flex flex-1 overflow-hidden md:hidden">
			{#if planningStore.mobileTab === 'backlog'}
				<div class="flex-1 overflow-hidden">
					<BacklogPanel />
				</div>
			{:else}
				<div class="flex-1 overflow-hidden">
					<WeeklyPlanningPanel />
				</div>
			{/if}
		</div>
	{/if}
</div>

{#if showStartConfirm}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<div
		class="fixed inset-0 z-[60] flex items-center justify-center bg-black/50"
		onclick={() => { if (!starting) showStartConfirm = false; }}
		onkeydown={(e) => { if (e.key === 'Escape' && !starting) showStartConfirm = false; }}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-sm rounded-lg border border-border bg-background p-6 shadow-xl"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="text-lg font-semibold text-foreground">{$t('planning.startConfirm')}</h3>
			<p class="mt-2 text-sm text-muted-foreground">
				{$t('planning.startDescription', { values: { label: planningStore.config?.weekly_label ?? '' } })}
			</p>
			<div class="mt-4 flex justify-end gap-2">
				<button
					class="rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
					onclick={() => { showStartConfirm = false; }}
					disabled={starting}
				>
					{$t('dialog.cancel')}
				</button>
				<button
					class="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
					onclick={handleStart}
					disabled={starting}
				>
					{starting ? $t('planning.starting') : $t('planning.start')}
				</button>
			</div>
		</div>
	</div>
{/if}
