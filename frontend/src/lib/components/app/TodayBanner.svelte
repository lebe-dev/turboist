<script lang="ts">
	import MarkdownText from '$lib/components/MarkdownText.svelte';
	import MegaphoneIcon from 'phosphor-svelte/lib/Megaphone';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { isBannerVisible } from '$lib/utils/banner';
	import { activeDayPart } from '$lib/utils/viewGroup';

	const text = $derived(settingsStore.bannerText);

	// A phase-scoped banner has to appear on its own, without a navigation or an
	// SSE refetch: re-read the clock every minute so the phase boundary lands.
	let now = $state(new Date());

	$effect(() => {
		if (settingsStore.bannerDayPart === '') return;
		const timer = window.setInterval(() => {
			now = new Date();
		}, 60_000);
		return () => window.clearInterval(timer);
	});

	const activePart = $derived(
		activeDayPart(now, configStore.value?.dayParts, configStore.value?.timezone ?? null)
	);
	const visible = $derived(
		isBannerVisible({
			published: settingsStore.bannerPublished,
			text,
			dayPart: settingsStore.bannerDayPart,
			activePart
		})
	);

	// The banner shimmers with a flowing gradient for 5s whenever it shows up —
	// on page load, or later when its day phase becomes active — then settles
	// into its normal static appearance.
	let shimmering = $state(true);

	$effect(() => {
		if (!visible) return;
		shimmering = true;
		const timer = setTimeout(() => {
			shimmering = false;
		}, 5000);
		return () => clearTimeout(timer);
	});
</script>

{#if visible}
	<div
		class="relative flex items-start gap-3 overflow-hidden border-b border-border bg-muted py-2 pl-3 pr-4 text-sm text-foreground sm:pl-4 sm:pr-6"
		role="status"
	>
		{#if shimmering}
			<div class="banner-shimmer pointer-events-none absolute inset-0" aria-hidden="true"></div>
		{/if}
		<MegaphoneIcon class="relative mt-0.5 size-4 shrink-0 text-muted-foreground" weight="fill" />
		<div class="relative min-w-0 flex-1 whitespace-pre-wrap break-words">
			<MarkdownText
				{text}
				linkClass="underline underline-offset-2 decoration-foreground/40 hover:decoration-foreground"
			/>
		</div>
	</div>
{/if}

<style>
	.banner-shimmer {
		/* light theme: red + white analog; overridden to black under .dark */
		--shimmer-accent: #ef4444;
		--shimmer-base: #ffffff;
		padding: 1px;
		background: linear-gradient(
			110deg,
			transparent 25%,
			var(--shimmer-accent) 42%,
			var(--shimmer-base) 50%,
			var(--shimmer-accent) 58%,
			transparent 75%
		);
		background-size: 200% 100%;
		/* show the gradient only along the outline (exclude the inner content box) */
		-webkit-mask:
			linear-gradient(#000 0 0) content-box,
			linear-gradient(#000 0 0);
		-webkit-mask-composite: xor;
		mask:
			linear-gradient(#000 0 0) content-box,
			linear-gradient(#000 0 0);
		mask-composite: exclude;
		opacity: 0.9;
		animation: banner-shimmer-sweep 5s linear 1 forwards;
	}

	:global(.dark) .banner-shimmer {
		--shimmer-base: #000000;
	}

	@keyframes banner-shimmer-sweep {
		0% {
			background-position: 250% 0;
			opacity: 0.9;
		}
		88% {
			opacity: 0.9;
		}
		100% {
			background-position: -250% 0;
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.banner-shimmer {
			animation: none;
			opacity: 0;
		}
	}
</style>
