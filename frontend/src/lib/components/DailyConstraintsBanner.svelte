<script lang="ts">
	import { constraintsStore } from '$lib/stores/constraints.svelte';
	import { t } from 'svelte-intl-precompile';
	import ShieldAlertIcon from '@lucide/svelte/icons/shield-alert';

	const constraints = $derived(constraintsStore.dailyConstraints);
	const visible = $derived(
		constraintsStore.enabled &&
		constraints.confirmed &&
		constraints.items.length > 0
	);
</script>

{#if visible}
	<div class="flex shrink-0 items-start gap-3 border-b border-red-500/30 bg-red-500/10 px-3 py-3 md:px-6">
		<ShieldAlertIcon class="mt-0.5 h-4 w-4 shrink-0 text-red-500/70" />
		<div class="flex-1">
			<span class="text-[12px] font-semibold uppercase tracking-wider text-red-500/80">{$t('constraints.dailyBannerTitle')}</span>
			<ul class="mt-1 space-y-0.5">
				{#each constraints.items as item (item)}
					<li class="text-[13px] text-red-500/70">- {item}</li>
				{/each}
			</ul>
		</div>
	</div>
{/if}
