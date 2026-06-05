<script lang="ts">
	// ResyncBanner is the F4.2 dismissible re-sync notice (US-4.2 AC4). When a
	// joined federated project has fallen behind the owner's retention, the
	// recovery loop re-bootstraps it from a fresh owner snapshot — preserving the
	// user's unsent local edits but possibly overriding them via per-field LWW. The
	// project DTO then carries reBootstrappedAt (the cutoff X). This banner surfaces
	// that X with the exact wording the plan pins: "your unsent changes from before
	// {X} were preserved but may have been overridden".
	//
	// Dismissal is local (per mount): the banner re-appears if the project is
	// re-bootstrapped AGAIN at a later cutoff, because the dismissed cutoff is
	// tracked and a NEW cutoff clears the dismissal. It renders nothing when the
	// project was never re-bootstrapped (reBootstrappedAt null).
	import { Button } from '$lib/components/ui/button';
	import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwise';
	import XIcon from 'phosphor-svelte/lib/X';
	import { t } from '$lib/i18n';
	import type { Project } from '$lib/api/types';

	let { project }: { project: Project } = $props();

	// dismissedCutoff tracks the cutoff X the user has dismissed. A later
	// re-bootstrap carries a newer cutoff, so the banner re-appears for it.
	let dismissedCutoff = $state<string | null>(null);

	const cutoff = $derived(project.reBootstrappedAt);
	const visible = $derived(cutoff !== null && cutoff !== dismissedCutoff);

	function fmtDate(iso: string): string {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return iso;
		return d.toLocaleString();
	}

	function dismiss(): void {
		dismissedCutoff = cutoff;
	}
</script>

{#if visible && cutoff}
	<div
		class="flex items-start gap-2 border-b border-amber-300/60 bg-amber-50 px-4 py-2 text-sm text-amber-900 sm:px-6 dark:border-amber-700/50 dark:bg-amber-950/40 dark:text-amber-200"
		role="status"
	>
		<ArrowsClockwiseIcon class="mt-0.5 size-4 shrink-0" />
		<div class="flex-1">
			<p class="font-medium">{$t('federation.resync.title')}</p>
			<p class="text-amber-800/90 dark:text-amber-200/80">
				{$t('federation.resync.body', { values: { date: fmtDate(cutoff) } })}
			</p>
		</div>
		<Button
			variant="ghost"
			size="sm"
			class="size-6 shrink-0 p-0 text-amber-900 hover:bg-amber-100 dark:text-amber-200 dark:hover:bg-amber-900/40"
			onclick={dismiss}
			aria-label={$t('federation.resync.dismiss')}
			title={$t('federation.resync.dismiss')}
		>
			<XIcon class="size-4" />
		</Button>
	</div>
{/if}
