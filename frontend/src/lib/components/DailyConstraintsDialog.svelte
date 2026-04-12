<script lang="ts">
	import { constraintsStore } from '$lib/stores/constraints.svelte';
	import { rollDailyConstraints, swapDailyConstraint, confirmDailyConstraints } from '$lib/api/client';
	import { toast } from 'svelte-sonner';
	import { t } from 'svelte-intl-precompile';
	import DicesIcon from '@lucide/svelte/icons/dices';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CheckIcon from '@lucide/svelte/icons/check';
	import ShuffleIcon from '@lucide/svelte/icons/shuffle';
	import { Button } from '$lib/components/ui/button/index.js';

	let { open = $bindable(false) }: { open: boolean } = $props();

	let rolling = $state(false);
	let swapping = $state<number | null>(null);
	let confirming = $state(false);

	const constraints = $derived(constraintsStore.dailyConstraints);
	const hasItems = $derived(constraints.items.length > 0);
	const canReroll = $derived(constraints.rerolls_used < constraints.max_rerolls);
	const isFirstRoll = $derived(constraints.needs_selection && !hasItems);

	async function handleRoll() {
		rolling = true;
		try {
			const res = await rollDailyConstraints();
			constraintsStore.updateDailyConstraints(res);
		} catch {
			toast.error($t('constraints.errorRoll'));
		} finally {
			rolling = false;
		}
	}

	async function handleSwap(index: number) {
		if (!canReroll) return;
		swapping = index;
		try {
			const res = await swapDailyConstraint(index);
			constraintsStore.updateDailyConstraints(res);
		} catch {
			toast.error($t('constraints.errorSwap'));
		} finally {
			swapping = null;
		}
	}

	async function handleConfirm() {
		confirming = true;
		try {
			const res = await confirmDailyConstraints();
			constraintsStore.updateDailyConstraints(res);
			open = false;
		} catch {
			toast.error($t('constraints.errorConfirm'));
		} finally {
			confirming = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (constraints.confirmed || !hasItems) {
				open = false;
			}
		}
	}

	function handleBackdropClick() {
		if (constraints.confirmed || !hasItems) {
			open = false;
		}
	}
</script>

{#if open}
	<div class="fixed inset-0 z-50 flex items-center justify-center">
		<!-- backdrop -->
		<button
			class="absolute inset-0 bg-background/80 backdrop-blur-sm"
			onclick={handleBackdropClick}
			tabindex="-1"
			aria-label="Close"
		></button>

		<!-- dialog -->
		<div
			class="relative z-10 mx-4 w-full max-w-md animate-fade-in-up rounded-xl border border-border bg-card p-6 shadow-lg"
			role="dialog"
			aria-modal="true"
		>
			<h2 class="mb-4 text-base font-semibold text-foreground">{$t('constraints.dailyTitle')}</h2>

			{#if isFirstRoll}
				<!-- Initial state: no items yet, need to roll -->
				<p class="mb-4 text-sm text-muted-foreground">
					{$t('constraints.dailyRerollsRemaining', { values: { count: constraints.max_rerolls } })}
				</p>
				<Button onclick={handleRoll} disabled={rolling} class="w-full">
					<DicesIcon class="mr-2 h-4 w-4" />
					{rolling ? '...' : $t('constraints.dailyRollAll')}
				</Button>
			{:else if hasItems}
				<!-- Show constraint items -->
				<ul class="mb-4 space-y-2">
					{#each constraints.items as item, i (i)}
						<li class="flex items-center gap-2 rounded-lg border border-border/50 bg-muted/30 px-3 py-2.5">
							<span class="flex-1 text-sm text-foreground">{item}</span>
							{#if !constraints.confirmed && canReroll}
								<button
									class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50"
									onclick={() => handleSwap(i)}
									disabled={swapping !== null}
									title={$t('constraints.dailySwap')}
								>
									{#if swapping === i}
										<div class="h-3 w-3 animate-spin rounded-full border-2 border-muted-foreground border-t-transparent"></div>
									{:else}
										<ShuffleIcon class="h-3.5 w-3.5" />
									{/if}
								</button>
							{/if}
						</li>
					{/each}
				</ul>

				<!-- Reroll info -->
				{#if !constraints.confirmed}
					<p class="mb-3 text-xs text-muted-foreground">
						{#if canReroll}
							{$t('constraints.dailyRerollsRemaining', { values: { count: constraints.max_rerolls - constraints.rerolls_used } })}
						{:else}
							{$t('constraints.dailyNoRerolls')}
						{/if}
					</p>
				{/if}

				<!-- Action buttons -->
				<div class="flex gap-2">
					{#if !constraints.confirmed}
						{#if canReroll}
							<Button variant="outline" onclick={handleRoll} disabled={rolling} class="flex-1">
								<RefreshCwIcon class="mr-2 h-3.5 w-3.5" />
								{rolling ? '...' : $t('constraints.dailyReroll')}
							</Button>
						{/if}
						<Button onclick={handleConfirm} disabled={confirming} class="flex-1">
							<CheckIcon class="mr-2 h-3.5 w-3.5" />
							{confirming ? '...' : $t('constraints.dailyConfirm')}
						</Button>
					{:else}
						<Button onclick={() => { open = false; }} class="w-full">
							OK
						</Button>
					{/if}
				</div>
			{/if}
		</div>
	</div>
{/if}

<svelte:window onkeydown={handleKeydown} />
