<script lang="ts">
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { isLabelVisible } from '$lib/utils/visibility';
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import { t } from '$lib/i18n';

	let { value = $bindable<number[]>([]) }: { value?: number[] } = $props();

	const labels = $derived(
		[...labelsStore.favourites, ...labelsStore.rest].filter((l) =>
			isLabelVisible(l, settingsStore.publicView)
		)
	);
	const selectedNames = $derived(
		value
			.map((id) => labels.find((l) => l.id === id)?.name)
			.filter((n): n is string => !!n)
	);

	function toggle(id: number): void {
		value = value.includes(id) ? value.filter((x) => x !== id) : [...value, id];
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class="inline-flex h-8 max-w-[16rem] items-center gap-1.5 rounded-md border border-border bg-background px-2.5 text-xs font-medium transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 data-[state=open]:bg-accent"
		aria-label={$t('common.labels')}
	>
		<TagIcon class="size-3.5 shrink-0" />
		<span class="truncate {selectedNames.length === 0 ? 'text-muted-foreground' : ''}">
			{selectedNames.length === 0 ? $t('common.labels') : selectedNames.join(', ')}
		</span>
		<CaretDownIcon class="size-3 shrink-0 text-muted-foreground" />
	</DropdownMenu.Trigger>
	<DropdownMenu.Content class="max-h-60 w-56 overflow-auto">
		{#if labels.length === 0}
			<div class="px-2 py-1.5 text-xs text-muted-foreground">
				{$t('settings.autoLabels.noLabelsAvailable')}
			</div>
		{:else}
			{#each labels as label (label.id)}
				<DropdownMenu.CheckboxItem
					checked={value.includes(label.id)}
					onCheckedChange={() => toggle(label.id)}
					closeOnSelect={false}
				>
					<span class="flex items-center gap-2">
						{#if label.color}
							<span class="size-2 rounded-full" style={`background-color: ${label.color}`}></span>
						{/if}
						{label.name}
					</span>
				</DropdownMenu.CheckboxItem>
			{/each}
		{/if}
	</DropdownMenu.Content>
</DropdownMenu.Root>
