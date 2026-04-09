<script lang="ts">
	let { class: className = '' }: { class?: string } = $props();
</script>

<svg
	viewBox="0 0 24 24"
	fill="currentColor"
	xmlns="http://www.w3.org/2000/svg"
	class={className}
>
	<!-- Three dots arranged in a triangle -->
	<circle cx="12" cy="6" r="3.5" />
	<circle cx="5.5" cy="17" r="3.5" />
	<circle cx="18.5" cy="17" r="3.5" />
</svg>
