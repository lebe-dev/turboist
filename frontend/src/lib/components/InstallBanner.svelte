<script lang="ts">
	import { onMount } from 'svelte';
	import { shouldShowIOSInstallBanner, dismissIOSInstallBanner } from '$lib/pwa/install.svelte';
	import { t } from 'svelte-intl-precompile';

	let show = $state(false);

	onMount(() => {
		show = shouldShowIOSInstallBanner();
	});

	function dismiss() {
		dismissIOSInstallBanner();
		show = false;
	}
</script>

{#if show}
	<div
		class="fixed bottom-4 left-1/2 z-50 flex -translate-x-1/2 items-center gap-3 rounded-lg border border-border bg-background px-4 py-3 shadow-lg"
	>
		<span class="text-sm text-foreground">{$t('pwa.installIOS')}</span>
		<button
			class="text-xs text-muted-foreground transition-colors hover:text-foreground"
			onclick={dismiss}
		>
			{$t('pwa.dismiss')}
		</button>
	</div>
{/if}
