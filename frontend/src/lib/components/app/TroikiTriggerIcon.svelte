<script lang="ts">
	let { class: className = '' }: { class?: string } = $props();
</script>

<svg
	xmlns="http://www.w3.org/2000/svg"
	viewBox="0 0 24 24"
	fill="currentColor"
	aria-hidden="true"
	class={className}
>
	<circle cx="12" cy="5" r="3" />
	<circle cx="6" cy="16" r="3" />
	<circle cx="18" cy="16" r="3" />
</svg>
