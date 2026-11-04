<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import XIcon from 'phosphor-svelte/lib/X';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import CircleIcon from 'phosphor-svelte/lib/Circle';
	import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircle';
	import ClockCounterClockwiseIcon from 'phosphor-svelte/lib/ClockCounterClockwise';
	import TrayIcon from 'phosphor-svelte/lib/Tray';
	import CalendarCheckIcon from 'phosphor-svelte/lib/CalendarCheck';
	import * as Kbd from '$lib/components/ui/kbd';
	import { getApiClient } from '$lib/api/client';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import type { DayPart, Project, Task, TaskInput } from '$lib/api/types';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { recentItemsStore, RECENT_ITEMS_SHOWN } from '$lib/stores/recentItems.svelte';
	import { recentProjectsStore } from '$lib/stores/recentProjects.svelte';
	import { inboxStatsStore } from '$lib/stores/inboxStats.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { nowStore } from '$lib/stores/now.svelte';
	import {
		matchRank,
		switcherResults,
		todayTargetProjects,
		type SwitcherResult
	} from '$lib/utils/quickSwitcher';
	import { nearestDayPart } from '$lib/utils/viewGroup';
	import { dayStartUtcInTz, toIsoUtc } from '$lib/utils/format';
	import { describeError } from '$lib/utils/taskActions';
	import { isMacOS } from '$lib/utils/platform';
	import { t } from '$lib/i18n';

	let { open = $bindable(false) }: { open?: boolean } = $props();

	/** Same floor as the server's search endpoint. */
	const MIN_QUERY = 2;
	const DEBOUNCE_MS = 150;
	const MAX_ROWS = 30;

	let query = $state('');
	let tasks = $state<Task[]>([]);
	let searching = $state(false);
	let active = $state(0);
	let creating = $state(false);
	let inputEl = $state<HTMLInputElement | null>(null);
	let listEl = $state<HTMLDivElement | null>(null);

	let timer: ReturnType<typeof setTimeout> | null = null;
	let seq = 0;

	const mod = isMacOS() ? '⌘' : 'Ctrl';

	// Projects are always in memory (the config aggregate, cached offline), so
	// they are matched locally — instant, and identical online and offline.
	const matchedProjects = $derived.by((): Project[] => {
		const q = query.trim();
		if (!q) return [];
		return projectsStore.items
			.map((p) => ({
				p,
				rank: matchRank(q, {
					title: p.title,
					labels: p.labels.map((l) => l.name),
					description: p.description
				})
			}))
			.filter((x) => x.rank !== null)
			.sort((a, b) => {
				if (a.rank !== b.rank) return (a.rank ?? 0) - (b.rank ?? 0);
				const ao = a.p.status === 'open' ? 0 : 1;
				const bo = b.p.status === 'open' ? 0 : 1;
				return ao - bo;
			})
			.map((x) => x.p);
	});

	const recents = $derived(
		query.trim() ? recentItemsStore.entries : recentItemsStore.entries.slice(0, RECENT_ITEMS_SHOWN)
	);

	const results = $derived(
		switcherResults(query, recents, tasks, matchedProjects).slice(0, MAX_ROWS)
	);

	const title = $derived(query.trim().replace(/\s+/g, ' '));
	// "For today" goes only into a project: the Inbox holds undated capture.
	const todayProjects = $derived(todayTargetProjects(recentProjectsStore.ids, projectsStore.items));
	// The phase a task written down now for today belongs to, so it does not land
	// in "anytime". Read from the wall clock (the shared now-store only ticks at
	// midnight), re-read as the panel opens and as the user types.
	function currentDayPart(): DayPart | null {
		return nearestDayPart(
			new Date(),
			configStore.value?.dayParts,
			configStore.value?.timezone ?? null
		);
	}
	const todayPart = $derived.by(() => {
		void open;
		void query;
		return currentDayPart();
	});

	type CreateTarget = { kind: 'inbox' } | { kind: 'today'; project: Project };

	type Row = { type: 'result'; result: SwitcherResult } | { type: 'create'; target: CreateTarget };

	const rows = $derived.by((): Row[] => {
		const out: Row[] = results.map((result) => ({ type: 'result', result }));
		if (title) {
			out.push({ type: 'create', target: { kind: 'inbox' } });
			for (const project of todayProjects) {
				out.push({ type: 'create', target: { kind: 'today', project } });
			}
		}
		return out;
	});

	$effect(() => {
		// Keep the highlight on a real row as the list changes under it.
		if (active >= rows.length) active = Math.max(0, rows.length - 1);
	});

	$effect(() => {
		if (open) return;
		query = '';
		tasks = [];
		active = 0;
		searching = false;
		if (timer) clearTimeout(timer);
		seq++;
	});

	function onInput(): void {
		active = 0;
		if (timer) clearTimeout(timer);
		const q = query.trim();
		if (q.length < MIN_QUERY) {
			seq++;
			tasks = [];
			searching = false;
			return;
		}
		searching = true;
		timer = setTimeout(() => void runSearch(q), DEBOUNCE_MS);
	}

	async function runSearch(q: string): Promise<void> {
		const mine = ++seq;
		try {
			const res = await viewsApi.search(getApiClient(), { q, type: 'tasks', limit: 50 });
			if (mine !== seq) return;
			tasks = res.tasks?.items ?? [];
		} catch {
			// Offline with nothing cached, or a failed request: the recents and the
			// local project matches still answer, so the list is simply shorter.
			if (mine !== seq) return;
			tasks = [];
		} finally {
			if (mine === seq) searching = false;
		}
	}

	function projectTitle(id: number | null): string | null {
		if (id === null) return null;
		return projectsStore.items.find((p) => p.id === id)?.title ?? null;
	}

	function subtitle(r: SwitcherResult): string | null {
		if (r.kind === 'project') return $t('quickSwitcher.kind.project');
		if (!r.task) return null;
		if (r.task.inboxId !== null) return $t('nav.inbox');
		return projectTitle(r.task.projectId);
	}

	function openResult(r: SwitcherResult): void {
		open = false;
		if (r.kind === 'project') {
			void goto(resolve('/(app)/project/[id]', { id: String(r.id) }));
		} else {
			void goto(resolve('/(app)/task/[id]', { id: String(r.id) }));
		}
	}

	function announce(task: Task): void {
		window.dispatchEvent(
			new CustomEvent('turboist:task-created', {
				detail: { task, projectId: task.projectId, contextId: task.contextId }
			})
		);
	}

	function todayDueAt(): string {
		return toIsoUtc(dayStartUtcInTz(nowStore.todayKey, configStore.value?.timezone ?? null));
	}

	async function create(target: CreateTarget | undefined): Promise<void> {
		if (!target || !title || creating) return;
		creating = true;
		const client = getApiClient();
		try {
			if (target.kind === 'inbox') {
				// The Inbox holds raw capture: no date, no phase.
				announce(await tasksApi.createInbox(client, { title }));
				toast.success($t('task.toast.addedToInbox'));
				void inboxStatsStore.load().catch(() => {});
			} else {
				const part = currentDayPart();
				const input: TaskInput = {
					title,
					dueAt: todayDueAt(),
					dueHasTime: false,
					...(part ? { dayPart: part } : {})
				};
				announce(await projectsApi.createTask(client, target.project.id, input));
				toast.success(
					$t('quickSwitcher.toast.addedToday', { values: { project: target.project.title } })
				);
			}
			open = false;
		} catch (err) {
			toast.error(describeError(err, $t('task.toast.failedAdd')));
		} finally {
			creating = false;
		}
	}

	function dayPartLabel(part: DayPart): string {
		return part === 'none' ? '' : $t(`task.dayPart.${part}`);
	}

	function activate(row: Row | undefined): void {
		if (!row) return;
		if (row.type === 'result') openResult(row.result);
		else void create(row.target);
	}

	function scrollActiveIntoView(): void {
		queueMicrotask(() => {
			listEl?.querySelector<HTMLElement>(`[data-row="${active}"]`)?.scrollIntoView({ block: 'nearest' });
		});
	}

	function onKeydown(e: KeyboardEvent): void {
		if (e.isComposing) return;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			if (rows.length) active = (active + 1) % rows.length;
			scrollActiveIntoView();
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			if (rows.length) active = (active - 1 + rows.length) % rows.length;
			scrollActiveIntoView();
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (e.shiftKey && (e.metaKey || e.ctrlKey)) {
				const first = todayProjects[0];
				void create(first ? { kind: 'today', project: first } : undefined);
			} else if (e.shiftKey) void create({ kind: 'inbox' });
			else activate(rows[active]);
		}
	}

	function clear(): void {
		query = '';
		onInput();
		inputEl?.focus();
	}
</script>

<DialogPrimitive.Root bind:open>
	<DialogPrimitive.Portal>
		<DialogPrimitive.Overlay
			class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
		/>
		<DialogPrimitive.Content
			onOpenAutoFocus={(e) => {
				e.preventDefault();
				inputEl?.focus();
			}}
			class="fixed left-1/2 top-[10%] z-50 flex max-h-[75vh] w-[calc(100%-2rem)] max-w-2xl -translate-x-1/2 flex-col overflow-hidden rounded-xl border border-border bg-popover text-popover-foreground shadow-xl outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
		>
			<DialogPrimitive.Title class="sr-only">{$t('quickSwitcher.title')}</DialogPrimitive.Title>
			<DialogPrimitive.Description class="sr-only">
				{$t('quickSwitcher.description')}
			</DialogPrimitive.Description>
			<div class="flex items-center gap-3 border-b border-border px-4 py-3">
				<MagnifyingGlassIcon class="size-4 shrink-0 text-muted-foreground" />
				<input
					bind:this={inputEl}
					bind:value={query}
					oninput={onInput}
					onkeydown={onKeydown}
					type="text"
					role="combobox"
					aria-expanded="true"
					aria-controls="quick-switcher-list"
					aria-activedescendant={rows.length ? `quick-switcher-row-${active}` : undefined}
					autocomplete="off"
					spellcheck="false"
					enterkeyhint="go"
					placeholder={$t('quickSwitcher.placeholder')}
					class="h-8 w-full bg-transparent text-base outline-none placeholder:text-muted-foreground"
				/>
				{#if query}
					<button
						type="button"
						onclick={clear}
						class="inline-flex size-6 shrink-0 items-center justify-center rounded-full bg-muted-foreground/30 text-background transition-colors hover:bg-muted-foreground/50"
						aria-label={$t('quickSwitcher.clear')}
					>
						<XIcon class="size-3.5" weight="bold" />
					</button>
				{/if}
			</div>

			<div
				bind:this={listEl}
				id="quick-switcher-list"
				role="listbox"
				class="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-2"
			>
				{#if !query.trim() && results.length > 0}
					<div class="px-2 pb-1 pt-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
						{$t('quickSwitcher.recent')}
					</div>
				{/if}
				{#each rows as row, i (row.type === 'result'
					? `${row.result.kind}:${row.result.id}`
					: row.target.kind === 'inbox'
						? 'create:inbox'
						: `create:today:${row.target.project.id}`)}
					{#if row.type === 'create' && row.target.kind === 'inbox' && results.length > 0}
						<div class="my-1 border-t border-border/60" role="separator" aria-hidden="true"></div>
					{/if}
					<button
						type="button"
						id="quick-switcher-row-{i}"
						data-row={i}
						role="option"
						aria-selected={i === active}
						disabled={row.type === 'create' && creating}
						onclick={() => activate(row)}
						onpointermove={() => (active = i)}
						class="flex w-full items-center gap-3 rounded-md px-3 py-2 text-left text-sm transition-colors disabled:opacity-60 {i === active
							? 'bg-accent text-accent-foreground'
							: ''}"
					>
						{#if row.type === 'result'}
							{@const r = row.result}
							{#if r.kind === 'project'}
								<FolderIcon class="size-4 shrink-0 text-muted-foreground" />
							{:else if r.task && r.task.status !== 'open'}
								<CheckCircleIcon class="size-4 shrink-0 text-muted-foreground" />
							{:else}
								<CircleIcon class="size-4 shrink-0 text-muted-foreground" />
							{/if}
							<span
								class="min-w-0 flex-1 truncate"
								class:line-through={r.task?.status === 'completed'}
								class:text-muted-foreground={r.task !== undefined && r.task.status !== 'open'}
							>
								{r.title}
							</span>
							{@const sub = subtitle(r)}
							{#if sub}
								<span class="max-w-[40%] shrink-0 truncate text-xs text-muted-foreground">{sub}</span>
							{/if}
							{#if r.recent}
								<ClockCounterClockwiseIcon
									class="size-3.5 shrink-0 text-muted-foreground"
									aria-label={$t('quickSwitcher.recentBadge')}
								/>
							{/if}
						{:else if row.target.kind === 'inbox'}
							<TrayIcon class="size-4 shrink-0 text-muted-foreground" />
							<span class="min-w-0 flex-1 truncate">
								{$t('quickSwitcher.create.inbox', { values: { title } })}
							</span>
							<span class="hidden shrink-0 items-center gap-1 sm:inline-flex">
								<Kbd.Kbd>⇧</Kbd.Kbd><Kbd.Kbd>↵</Kbd.Kbd>
							</span>
						{:else}
							{@const target = row.target}
							<CalendarCheckIcon class="size-4 shrink-0 text-muted-foreground" />
							<span class="min-w-0 flex-1 truncate">
								{$t('quickSwitcher.create.today', { values: { title, project: target.project.title } })}
							</span>
							{#if todayPart}
								<span class="shrink-0 text-xs text-muted-foreground">{dayPartLabel(todayPart)}</span>
							{/if}
							{#if target.project.id === todayProjects[0]?.id}
								<span class="hidden shrink-0 items-center gap-1 sm:inline-flex">
									<Kbd.Kbd>{mod}</Kbd.Kbd><Kbd.Kbd>⇧</Kbd.Kbd><Kbd.Kbd>↵</Kbd.Kbd>
								</span>
							{/if}
						{/if}
					</button>
				{/each}
				{#if rows.length === 0}
					<div class="px-2 py-6 text-center text-xs text-muted-foreground">
						{$t('quickSwitcher.empty')}
					</div>
				{:else if searching && query.trim().length >= MIN_QUERY}
					<div class="px-3 py-1 text-xs text-muted-foreground">{$t('quickSwitcher.searching')}</div>
				{/if}
			</div>

			<div
				class="hidden shrink-0 items-center gap-4 border-t border-border px-4 py-2 text-xs text-muted-foreground sm:flex"
			>
				<span class="inline-flex items-center gap-1"><Kbd.Kbd>↑</Kbd.Kbd><Kbd.Kbd>↓</Kbd.Kbd> {$t('quickSwitcher.hint.navigate')}</span>
				<span class="inline-flex items-center gap-1"><Kbd.Kbd>↵</Kbd.Kbd> {$t('quickSwitcher.hint.open')}</span>
				<span class="inline-flex items-center gap-1"><Kbd.Kbd>esc</Kbd.Kbd> {$t('quickSwitcher.hint.close')}</span>
			</div>
		</DialogPrimitive.Content>
	</DialogPrimitive.Portal>
</DialogPrimitive.Root>
