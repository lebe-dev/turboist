<script lang="ts">
	import MarkdownText from '$lib/components/MarkdownText.svelte';
	import MegaphoneIcon from 'phosphor-svelte/lib/Megaphone';
	import { settingsStore } from '$lib/stores/settings.svelte';

	const text = $derived(settingsStore.bannerText);
	const visible = $derived(settingsStore.bannerPublished && text.trim() !== '');
</script>

{#if visible}
	<div
		class="flex items-start gap-3 border-b border-border bg-muted px-4 py-2 text-sm text-foreground sm:px-6"
		role="status"
	>
		<MegaphoneIcon class="mt-0.5 size-4 shrink-0 text-muted-foreground" weight="fill" />
		<div class="min-w-0 flex-1 whitespace-pre-wrap break-words">
			<MarkdownText
				{text}
				linkClass="underline underline-offset-2 decoration-foreground/40 hover:decoration-foreground"
			/>
		</div>
	</div>
{/if}
