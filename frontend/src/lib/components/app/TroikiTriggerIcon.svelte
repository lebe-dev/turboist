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
	<circle cx="12" cy="4.5" r="2" />
	<circle cx="5.5" cy="15.75" r="2" />
	<circle cx="18.5" cy="15.75" r="2" />
</svg>
