<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { SvelteSet } from 'svelte/reactivity';
	import { outboxStatusStore } from '$lib/offline/outboxStatus.svelte';
	import { t } from '$lib/i18n';

	const shown = new SvelteSet<string>();

	$effect(() => {
		const entries = outboxStatusStore.failed;
		const ids = new Set(entries.map((e) => e.id));
		for (const id of shown) {
			if (!ids.has(id)) shown.delete(id);
		}
		for (const entry of entries) {
			if (shown.has(entry.id)) continue;
			shown.add(entry.id);
			toast.error($t('offline.failed.title'), {
				description: entry.lastError ?? $t('offline.failed.unknown'),
				duration: Infinity,
				action: {
					label: $t('offline.failed.retry'),
					onClick: () => {
						shown.delete(entry.id);
						void outboxStatusStore.retry(entry.id);
					}
				},
				cancel: {
					label: $t('offline.failed.discard'),
					onClick: () => {
						void outboxStatusStore.discard(entry.id);
					}
				}
			});
		}
	});
</script>
