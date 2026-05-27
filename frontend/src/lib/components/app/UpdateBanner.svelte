<script lang="ts">
	import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwise';
	import { useRegisterSW } from 'virtual:pwa-register/svelte';
	import { t } from '$lib/i18n';

	const { needRefresh, updateServiceWorker } = useRegisterSW({
		onRegisteredSW(_url, registration) {
			if (!registration) return;
			setInterval(() => registration.update(), 60 * 60 * 1000);
		}
	});

	function update(): void {
		void updateServiceWorker(true);
	}
</script>

{#if $needRefresh}
	<div
		class="flex items-center gap-2 border-b border-blue-500/30 bg-blue-500/10 px-4 py-1.5 text-[12px] text-blue-700 dark:text-blue-300"
		role="status"
		aria-live="polite"
		data-testid="update-banner"
	>
		<ArrowsClockwiseIcon class="size-3.5" />
		<span>{$t('app.updateAvailable')}</span>
		<button
			onclick={update}
			class="ml-auto rounded px-2 py-0.5 font-medium hover:bg-blue-500/20 transition-colors"
		>
			{$t('app.updateNow')}
		</button>
	</div>
{/if}
