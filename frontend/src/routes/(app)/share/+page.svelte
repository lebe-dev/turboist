<script lang="ts">
	// Web Share Target (static/manifest.webmanifest → share_target): the
	// installed PWA appears in the system share sheet, and a share lands here as
	// GET /share?title=…&text=…&url=…. It opens QuickAdd prefilled — a bare link
	// becomes the title, so the server's link-title job can name the task.
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { requestQuickAdd } from '$lib/native/deepLink';
	import { prefillFromShare } from '$lib/utils/sharedContent';

	onMount(() => {
		const params = page.url.searchParams;
		const prefill = prefillFromShare({
			title: params.get('title'),
			text: params.get('text'),
			url: params.get('url')
		});
		void goto(resolve('/inbox'), { replaceState: true }).then(() => {
			if (prefill) requestQuickAdd(prefill);
		});
	});
</script>

<div class="p-6 text-sm text-muted-foreground">{$t('app.redirecting')}</div>
