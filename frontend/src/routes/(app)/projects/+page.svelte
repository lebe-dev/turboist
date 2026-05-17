<script lang="ts">
	import { resolve } from '$app/paths';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import TroikiTriggerIcon from '$lib/components/app/TroikiTriggerIcon.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { isProjectVisible } from '$lib/utils/visibility';
	import type { Project, ProjectStatus } from '$lib/api/types';
	import { t } from '$lib/i18n';

	type Filter = 'all' | 'archived' | 'cancelled' | 'completed' | 'generic' | 'software';

	const FILTERS: Array<{ value: Filter; labelKey: string }> = [
		{ value: 'all', labelKey: 'page.projects.filterAll' },
		{ value: 'generic', labelKey: 'page.projects.filterGeneric' },
		{ value: 'software', labelKey: 'page.projects.filterSoftware' },
		{ value: 'archived', labelKey: 'page.projects.filterArchived' },
		{ value: 'cancelled', labelKey: 'page.projects.filterCancelled' },
		{ value: 'completed', labelKey: 'page.projects.filterCompleted' }
	];

	const STATUS_KEY: Record<ProjectStatus, string> = {
		open: 'project.statusOpen',
		completed: 'project.statusCompleted',
		archived: 'project.statusArchived',
		cancelled: 'project.statusCancelled'
	};

	const TROIKI_ORDER: Record<'important' | 'medium' | 'rest', number> = {
		important: 0,
		medium: 1,
		rest: 2
	};

	let activeFilter = $state<Filter>('all');

	const contextsById = $derived.by(() => {
		const map: Record<number, string> = {};
		for (const c of contextsStore.items) map[c.id] = c.name;
		return map;
	});

	const visible = $derived(
		projectsStore.items.filter((p) => isProjectVisible(p, settingsStore.publicView))
	);

	const counts = $derived({
		all: visible.length,
		generic: visible.filter((p) => p.projectType === 'generic').length,
		software: visible.filter((p) => p.projectType === 'software').length,
		archived: visible.filter((p) => p.status === 'archived').length,
		cancelled: visible.filter((p) => p.status === 'cancelled').length,
		completed: visible.filter((p) => p.status === 'completed').length
	});

	const filtered = $derived.by<Project[]>(() => {
		const list =
			activeFilter === 'all'
				? visible
				: activeFilter === 'generic' || activeFilter === 'software'
					? visible.filter((p) => p.projectType === activeFilter)
					: visible.filter((p) => p.status === activeFilter);
		return [...list].sort((a, b) => {
			const aOpen = a.status === 'open';
			const bOpen = b.status === 'open';
			if (aOpen !== bOpen) return aOpen ? -1 : 1;
			if (activeFilter === 'all' || activeFilter === 'generic' || activeFilter === 'software') {
				const ta = a.troikiCategory;
				const tb = b.troikiCategory;
				if (ta && !tb) return -1;
				if (!ta && tb) return 1;
				if (ta && tb && ta !== tb) return TROIKI_ORDER[ta] - TROIKI_ORDER[tb];
			}
			return a.title.localeCompare(b.title);
		});
	});
</script>

<ViewHeader title={$t('page.projects.title')}>
	{#snippet banner()}
		<div class="flex flex-wrap items-center gap-2 px-1 pb-1">
			{#each FILTERS as f (f.value)}
				{@const active = activeFilter === f.value}
				{@const count = counts[f.value]}
				<Button
					size="sm"
					variant={active ? 'secondary' : 'ghost'}
					onclick={() => (activeFilter = f.value)}
				>
					{$t(f.labelKey)}
					<Badge variant="outline" class="ml-1 h-4 text-[10px]">{count}</Badge>
				</Button>
			{/each}
		</div>
	{/snippet}
</ViewHeader>

<div class="px-2 py-2 sm:px-6">
	<ViewContent
		loading={!projectsStore.loaded}
		isEmpty={filtered.length === 0}
		emptyIcon={FolderIcon}
		emptyTitle={$t('page.projects.emptyTitle')}
		emptyDescription={$t('page.projects.emptyDescription')}
	>
		<ul class="flex flex-col divide-y divide-border/50 overflow-hidden rounded-md border border-border/60 bg-card">
			{#each filtered as project (project.id)}
				{@const href = resolve('/(app)/project/[id]', { id: String(project.id) })}
				{@const ctxName = contextsById[project.contextId]}
				<li>
					<a
						{href}
						class="flex items-center gap-3 px-3 py-2.5 text-sm transition-colors hover:bg-muted/40"
					>
						<FolderIcon
							class="size-4 shrink-0 opacity-90"
							style={`color: ${project.color}`}
							weight="fill"
						/>
						<span class="min-w-0 flex-1 truncate font-medium text-foreground">
							{project.title}
						</span>
						{#if project.isPrivate && !settingsStore.publicView}
							<span
								class="inline-flex shrink-0"
								title={$t('common.privateTooltip')}
								aria-label={$t('common.privateMarker')}
							>
								<LockSimpleIcon class="size-3 text-muted-foreground/50" />
							</span>
						{/if}
						{#if project.troikiCategory}
							<TroikiTriggerIcon class="size-3 shrink-0 text-muted-foreground/60" />
						{/if}
						{#if ctxName}
							<span class="shrink-0 truncate text-xs text-muted-foreground">{ctxName}</span>
						{/if}
						{#if project.status !== 'open'}
							<Badge variant="outline" class="shrink-0">
								{$t(STATUS_KEY[project.status])}
							</Badge>
						{/if}
					</a>
				</li>
			{/each}
		</ul>
	</ViewContent>
</div>
