<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Spinner } from '$lib/components/ui/spinner';

	// Native-only pull-to-refresh: dragging down from the top of the scroll
	// container with elastic resistance triggers `onRefresh`. Web has no
	// equivalent gesture wired up — desktop/mobile browsers already offer their
	// own overscroll behavior, and the app stays fresh there via SSE.
	let {
		children,
		onRefresh,
		tag = 'div',
		class: className = ''
	}: {
		children: Snippet;
		onRefresh: () => Promise<void>;
		tag?: string;
		class?: string;
	} = $props();

	const THRESHOLD = 64;
	const MAX_PULL = 100;

	let containerEl = $state<HTMLElement | null>(null);
	let pullDistance = $state(0);
	let refreshing = $state(false);
	let armed = false;
	let startY = 0;

	// Diminishing pull the further the finger travels, capped at MAX_PULL —
	// gives the drag an elastic feel instead of tracking 1:1 with the finger.
	function resistance(delta: number): number {
		return Math.min(MAX_PULL, delta * 0.45);
	}

	function onTouchStart(e: TouchEvent): void {
		if (refreshing || !containerEl || containerEl.scrollTop > 0) {
			armed = false;
			return;
		}
		startY = e.touches[0].clientY;
		armed = true;
	}

	function onTouchMove(e: TouchEvent): void {
		if (!armed) return;
		// A sibling gesture (e.g. task drag-reorder) already claimed this touch.
		if (e.defaultPrevented) {
			armed = false;
			pullDistance = 0;
			return;
		}
		const delta = e.touches[0].clientY - startY;
		if (delta <= 0 || !containerEl || containerEl.scrollTop > 0) {
			armed = false;
			pullDistance = 0;
			return;
		}
		pullDistance = resistance(delta);
		e.preventDefault();
	}

	async function onTouchEnd(): Promise<void> {
		if (!armed) return;
		armed = false;
		if (pullDistance < THRESHOLD) {
			pullDistance = 0;
			return;
		}
		refreshing = true;
		pullDistance = THRESHOLD;
		try {
			await onRefresh();
		} finally {
			refreshing = false;
			pullDistance = 0;
		}
	}
</script>

<!-- The touch handlers augment scrolling (an elastic pull gesture), they don't
	 change the element's semantics — `tag` already carries its own role (e.g.
	 the `main` landmark), so no separate interactive role applies here. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<svelte:element
	this={tag}
	bind:this={containerEl}
	class={className}
	style:overscroll-behavior-y="contain"
	ontouchstart={onTouchStart}
	ontouchmove={onTouchMove}
	ontouchend={onTouchEnd}
	ontouchcancel={onTouchEnd}
>
	<div
		class="flex items-center justify-center overflow-hidden transition-[height] duration-200 ease-out"
		style:height="{pullDistance}px"
	>
		<Spinner
			class="size-5 text-muted-foreground {refreshing ? '' : 'animate-none'}"
			style={refreshing ? '' : `transform: rotate(${Math.min(180, (pullDistance / THRESHOLD) * 180)}deg)`}
		/>
	</div>
	{@render children()}
</svelte:element>
