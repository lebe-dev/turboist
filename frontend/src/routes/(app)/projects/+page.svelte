<script lang="ts">
	import { resolve } from '$app/paths';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import CaretRightIcon from 'phosphor-svelte/lib/CaretRight';
	import { getApiClient } from '$lib/api/client';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { useInvalidation } from '$lib/hooks/useInvalidation.svelte';
	import ProjectRadarChart from '$lib/components/project/ProjectRadarChart.svelte';
	import { buildProjectRadar } from '$lib/utils/projectStats';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import TroikiTriggerIcon from '$lib/components/app/TroikiTriggerIcon.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { isProjectVisible } from '$lib/utils/visibility';
	import type {
		LabelStatsPeriod,
		Project,
		ProjectStatsResponse,
		ProjectStatus
	} from '$lib/api/types';
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

	const PERIODS: LabelStatsPeriod[] = ['week', 'month', 'quarter'];
	const PERIOD_LABEL: Record<LabelStatsPeriod, string> = {
		week: 'page.projects.radar.period.week',
		month: 'page.projects.radar.period.month',
		quarter: 'page.projects.radar.period.quarter'
	};

	let activeFilter = $state<Filter>('all');
	let searchQuery = $state('');

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

	// The activity radar is collapsed until asked for: it costs a request nobody
	// who only wants the list should pay, so the report loads on first expand.
	let radarOpen = $state(false);
	let radarPeriod = $state<LabelStatsPeriod>('week');
	let stats = $state<ProjectStatsResponse | null>(null);

	const radar = $derived(stats ? buildProjectRadar(visible, stats.items, radarPeriod) : null);

	const radarLoader = usePageLoad(
		async (isValid) => {
			const res = await projectsApi.stats(getApiClient());
			if (!isValid()) return;
			stats = res;
		},
		{ autoLoad: false, errorMessage: $t('page.projects.radar.errorLoading') }
	);

	function toggleRadar() {
		radarOpen = !radarOpen;
		if (radarOpen && stats === null) void radarLoader.refetch();
	}

	// Finishing a task moves the numbers; refresh in place, but only while the
	// radar is open — a collapsed one has nothing on screen to keep current.
	useInvalidation(['tasks', 'projects'], () => {
		if (radarOpen) void radarLoader.revalidate();
	});

	const searched = $derived.by<Project[]>(() => {
		const q = searchQuery.trim().toLowerCase();
		if (!q) return filtered;
		return filtered.filter((p) => p.title.toLowerCase().includes(q));
	});
</script>

<ViewHeader title={$t('page.projects.title')}>
	{#snippet banner()}
		<div class="flex flex-col gap-2 px-1 pb-1">
			<div class="flex flex-wrap items-center gap-2">
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
			<div class="relative">
				<MagnifyingGlassIcon
					class="absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
				/>
				<input
					type="text"
					bind:value={searchQuery}
					placeholder={$t('page.projects.searchPlaceholder')}
					class="h-8 w-full rounded-md border border-border bg-background pr-3 pl-8 text-xs outline-none placeholder:text-muted-foreground focus:ring-2 focus:ring-ring/50"
				/>
			</div>
		</div>
	{/snippet}
</ViewHeader>

<div class="px-2 py-2 sm:px-6">
	<section class="mb-3 rounded-md border border-border/60 bg-card" data-testid="projects-radar">
		<button
			type="button"
			onclick={toggleRadar}
			aria-expanded={radarOpen}
			aria-controls="projects-radar-body"
			class="flex w-full items-center gap-2 px-3 py-2.5 text-left"
		>
			<CaretRightIcon
				class="size-3.5 shrink-0 text-muted-foreground transition-transform {radarOpen
					? 'rotate-90'
					: ''}"
				weight="bold"
			/>
			<span class="text-sm font-semibold tracking-wide text-muted-foreground uppercase">
				{$t('page.projects.radar.title')}
			</span>
			<span class="sr-only">
				{radarOpen ? $t('page.projects.radar.collapse') : $t('page.projects.radar.expand')}
			</span>
		</button>
		{#if radarOpen}
			<div id="projects-radar-body" class="flex flex-col gap-3 border-t border-border/60 p-4">
				<div class="flex flex-wrap items-center justify-between gap-2">
					<div
						class="flex items-center gap-1 rounded-md border border-border bg-card p-0.5"
						role="radiogroup"
						aria-label={$t('page.projects.radar.periodAria')}
					>
						{#each PERIODS as value (value)}
							{@const active = radarPeriod === value}
							<button
								type="button"
								role="radio"
								aria-checked={active}
								onclick={() => (radarPeriod = value)}
								class="rounded px-2.5 py-1 text-[13px] transition-colors {active
									? 'bg-muted font-medium text-foreground'
									: 'text-muted-foreground hover:text-foreground'}"
							>
								{$t(PERIOD_LABEL[value])}
							</button>
						{/each}
					</div>
					<span class="text-xs text-muted-foreground">{$t('page.projects.radar.hint')}</span>
				</div>
				{#if radar}
					<ProjectRadarChart {radar} />
				{:else if radarLoader.error}
					<p class="py-6 text-center text-sm text-muted-foreground">{radarLoader.error}</p>
				{:else if stats === null}
					<p class="py-6 text-center text-sm text-muted-foreground">…</p>
				{:else}
					<p class="py-6 text-center text-sm text-muted-foreground">
						{$t('page.projects.radar.empty')}
					</p>
				{/if}
			</div>
		{/if}
	</section>

	<ViewContent
		loading={!projectsStore.loaded}
		isEmpty={searched.length === 0}
		emptyIcon={FolderIcon}
		emptyTitle={$t('page.projects.emptyTitle')}
		emptyDescription={$t('page.projects.emptyDescription')}
	>
		<ul
			class="flex flex-col divide-y divide-border/50 overflow-hidden rounded-md border border-border/60 bg-card"
		>
			{#each searched as project (project.id)}
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
						{#if settingsStore.troikiEnabled && project.troikiCategory}
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
