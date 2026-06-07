<script lang="ts">
	// PeerStatusBadge renders a peer's federation link health as a semantic status
	// chip (Federation v1 F1.4, US-1.4 AC3). The key intent is tone: a healthy
	// "active" link reads as calm/neutral — a grey chip (light-grey on dark, dark-grey
	// on light) with a live pulsing dot — NOT the brand-primary red that previously
	// made a perfectly healthy peer look like an alarm. Only genuinely
	// terminal/destructive states (revoked) borrow the destructive red; warnings
	// (stale) use amber; quiet states (paused/left) sit in the muted palette.
	//
	// The visible label is a direct text child of the pill (the dot is an empty
	// sibling element), so the status text node stays isolated — callers/tests can
	// match the label exactly (e.g. /^Active$/).
	import { t } from '$lib/i18n';
	import type { PeerStatus } from '$lib/api/types';

	let { status }: { status: PeerStatus } = $props();

	// Per-status pill tone (border + tint + text) and the status dot colour. Active
	// additionally pulses to signal a live, flowing link.
	const TONE: Record<PeerStatus, { pill: string; dot: string; pulse: boolean }> = {
		active: {
			pill: 'border-border bg-muted text-foreground/75',
			dot: 'bg-foreground/60',
			pulse: true
		},
		paused: {
			pill: 'border-border bg-muted text-muted-foreground',
			dot: 'bg-muted-foreground/60',
			pulse: false
		},
		stale: {
			pill: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
			dot: 'bg-amber-500',
			pulse: false
		},
		revoked: {
			pill: 'border-destructive/25 bg-destructive/10 text-destructive',
			dot: 'bg-destructive',
			pulse: false
		},
		left: {
			pill: 'border-border bg-transparent text-muted-foreground',
			dot: 'bg-muted-foreground/40',
			pulse: false
		}
	};

	const tone = $derived(TONE[status]);
	const label = $derived($t(`federation.peers.status.${status}`));
</script>

<span
	class={[
		'inline-flex shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium leading-none',
		tone.pill
	]}
>
	<span class="relative flex size-1.5 shrink-0">
		{#if tone.pulse}
			<span class={['absolute inline-flex size-full animate-ping rounded-full opacity-60', tone.dot]}></span>
		{/if}
		<span class={['relative inline-flex size-1.5 rounded-full', tone.dot]}></span>
	</span>{label}
</span>
