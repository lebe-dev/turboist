<script lang="ts">
	import { goto } from '$app/navigation';
	import { appStore } from '$lib/stores/app.svelte';
	import { labelFilterStore } from '$lib/stores/label-filter.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import TagIcon from '@lucide/svelte/icons/tag';
	import { t } from 'svelte-intl-precompile';

	const labels = $derived(appStore.labels);

	function selectLabel(name: string) {
		labelFilterStore.set(name);
		contextsStore.setView('all');
		goto('/');
	}
</script>

<div class="flex h-full flex-col">
	<header class="flex h-12 shrink-0 items-center border-b border-border/50 px-6">
		<h1 class="text-sm font-semibold tracking-wide text-foreground">{$t('sidebar.labels')}</h1>
	</header>

	<div class="flex-1 overflow-y-auto px-3 py-3 md:px-6">
		{#if labels.length === 0}
			<p class="py-8 text-center text-sm text-muted-foreground/60">{$t('task.noLabelsFound')}</p>
		{:else}
			<div class="flex flex-wrap gap-2">
				{#each labels as label (label.id)}
					<button
						class="group flex items-center gap-2 rounded-lg border border-border/50 px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
						onclick={() => selectLabel(label.name)}
					>
						<TagIcon
							class="h-3.5 w-3.5 shrink-0 opacity-60"
							style={label.color ? `color: ${label.color}` : ''}
						/>
						<span>{label.name}</span>
					</button>
				{/each}
			</div>
		{/if}
	</div>
</div>
