<script lang="ts">
	let { children } = $props();
</script>

<div class="flex min-h-screen items-center justify-center bg-background p-6">
	<div class="w-full max-w-sm">
		<div class="mb-8 text-center">
			<h1
				class="text-4xl font-medium tracking-tight text-foreground"
				style="font-family: var(--font-display)"
			>
				Turboist
			</h1>
			<p class="mt-1.5 text-sm text-muted-foreground">Task management for the rest of us</p>
		</div>
		{@render children()}
	</div>
</div>
