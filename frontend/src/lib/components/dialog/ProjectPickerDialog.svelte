<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import PushPinIcon from 'phosphor-svelte/lib/PushPin';
	import { t } from '$lib/i18n';

	let {
		open = $bindable(false),
		title = null,
		onSelect
	}: {
		open?: boolean;
		title?: string | null;
		onSelect?: (projectId: number) => void;
	} = $props();

	let query = $state('');

	const visibleProjects = $derived(
		projectsStore.items
			.filter((p) => p.status !== 'completed')
			.filter((p) => {
				const ctx = userStateStore.activeContextId;
				return ctx === null || p.contextId === ctx;
			})
			.slice()
			.sort((a, b) => {
				if (a.isPinned !== b.isPinned) return a.isPinned ? -1 : 1;
				return a.title.localeCompare(b.title);
			})
	);
	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return visibleProjects;
		return visibleProjects.filter((p) => p.title.toLowerCase().includes(q));
	});

	function pick(id: number): void {
		onSelect?.(id);
		open = false;
	}

	$effect(() => {
		if (!open) query = '';
	});
</script>

<DialogPrimitive.Root bind:open>
	<DialogPrimitive.Portal>
		<DialogPrimitive.Overlay
			class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
		/>
		<DialogPrimitive.Content
			class="fixed left-1/2 top-[15%] z-50 flex max-h-[70vh] w-[calc(100%-2rem)] max-w-sm -translate-x-1/2 flex-col overflow-hidden rounded-xl border border-border bg-popover text-popover-foreground shadow-xl outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
		>
			<DialogPrimitive.Title class="shrink-0 px-4 pt-4 text-sm font-semibold">
				{title ?? $t('template.pickProject.title')}
			</DialogPrimitive.Title>
			<DialogPrimitive.Description class="sr-only">
				{$t('template.pickProject.description')}
			</DialogPrimitive.Description>
			<div class="mt-3 flex items-center gap-2 border-b border-border px-4 py-2">
				<MagnifyingGlassIcon class="size-3.5 text-muted-foreground" />
				<!-- svelte-ignore a11y_autofocus -->
				<input
					bind:value={query}
					type="text"
					placeholder={$t('dialog.quickAdd.searchProjectsPlaceholder')}
					class="h-6 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
					autofocus
				/>
			</div>
			<div class="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-2">
				{#each filtered as project (project.id)}
					<button
						type="button"
						onclick={() => pick(project.id)}
						class="inline-flex items-center gap-2 rounded px-2 py-2 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
					>
						{#if project.isPinned}
							<PushPinIcon class="size-3 shrink-0 text-amber-500/80" weight="fill" />
						{/if}
						<span class="flex-1 truncate">{project.title}</span>
					</button>
				{/each}
				{#if filtered.length === 0}
					<div class="px-2 py-6 text-center text-xs text-muted-foreground">
						{$t('dialog.quickAdd.noMatches')}
					</div>
				{/if}
			</div>
		</DialogPrimitive.Content>
	</DialogPrimitive.Portal>
</DialogPrimitive.Root>
