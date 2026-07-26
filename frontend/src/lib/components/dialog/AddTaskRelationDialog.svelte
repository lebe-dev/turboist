<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import { onDestroy } from 'svelte';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircle';
	import { getApiClient } from '$lib/api/client';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import type { RelationDirection, RelationType, Task } from '$lib/api/types';
	import { describeError } from '$lib/utils/taskActions';
	import { t } from '$lib/i18n';

	let {
		open = $bindable(false),
		currentTaskId,
		excludeTaskIds = [],
		onSelect
	}: {
		open?: boolean;
		currentTaskId: number;
		/** Peers already related to this task — offered but marked as taken. */
		excludeTaskIds?: number[];
		onSelect?: (
			targetTaskId: number,
			type: RelationType,
			direction: RelationDirection
		) => void | Promise<void>;
	} = $props();

	// The three user-facing choices collapse onto (type, direction): `related` is
	// symmetric so its direction is irrelevant, `blocks` needs one.
	type Choice = { key: string; type: RelationType; direction: RelationDirection };
	const CHOICES: Choice[] = [
		{ key: 'blockedBy', type: 'blocks', direction: 'incoming' },
		{ key: 'blocks', type: 'blocks', direction: 'outgoing' },
		{ key: 'related', type: 'related', direction: 'outgoing' }
	];

	let choiceKey = $state('blockedBy');
	const choice = $derived(CHOICES.find((c) => c.key === choiceKey) ?? CHOICES[0]);

	let query = $state('');
	let results = $state<Task[]>([]);
	let searching = $state(false);
	let searchError = $state<string | null>(null);
	let timer: ReturnType<typeof setTimeout> | undefined;

	const excluded = $derived(new Set(excludeTaskIds));
	// The current task can never be its own peer, and an existing peer would only be
	// rejected as a duplicate — filter both out rather than letting the user hit a 409.
	const selectable = $derived(
		results.filter((task) => task.id !== currentTaskId && !excluded.has(task.id))
	);

	// The backend requires q >= 2 characters (GET /api/v1/search), so shorter text is
	// not worth a round-trip. A pure-digit term is exempt: it is looked up by id.
	const MIN_QUERY = 2;

	/** The term as a task id, or null when it is not a plain positive integer. */
	function asTaskId(term: string): number | null {
		if (!/^\d+$/.test(term)) return null;
		const id = Number(term);
		return Number.isSafeInteger(id) && id > 0 ? id : null;
	}

	/** Whether a term is worth a request at all. */
	function isSearchable(term: string): boolean {
		return term.length >= MIN_QUERY || asTaskId(term) !== null;
	}

	async function runSearch(term: string): Promise<void> {
		searching = true;
		searchError = null;
		const client = getApiClient();
		const id = asTaskId(term);
		try {
			// A digit term is looked up by id AND searched as text: "9" can be an id and
			// also occur in a title, and only the user knows which they meant. The id hit
			// leads. Reuses the plain task GET — no lookup endpoint of its own.
			const [byId, byText] = await Promise.all([
				id === null
					? Promise.resolve(null)
					: // A missing id is "no match", not a failure, so it must not surface as an
						// error — the text arm may still have something.
						tasksApi.get(client, id).catch(() => null),
				term.length < MIN_QUERY
					? Promise.resolve<Task[]>([])
					: viewsApi
							.search(client, { q: term, type: 'tasks', limit: 20 })
							.then((res) => res.tasks?.items ?? [])
			]);
			results = byId ? [byId, ...byText.filter((task) => task.id !== byId.id)] : byText;
		} catch (err) {
			results = [];
			searchError = describeError(err, $t('page.task.relationSearchFailed'));
		} finally {
			searching = false;
		}
	}

	function onInput(e: Event): void {
		query = (e.currentTarget as HTMLInputElement).value;
		if (timer) clearTimeout(timer);
		const term = query.trim();
		if (!isSearchable(term)) {
			results = [];
			searchError = null;
			return;
		}
		timer = setTimeout(() => void runSearch(term), 250);
	}

	async function pick(task: Task): Promise<void> {
		await onSelect?.(task.id, choice.type, choice.direction);
		open = false;
	}

	$effect(() => {
		if (open) return;
		if (timer) clearTimeout(timer);
		query = '';
		results = [];
		searchError = null;
		choiceKey = 'blockedBy';
	});

	onDestroy(() => {
		if (timer) clearTimeout(timer);
	});
</script>

<DialogPrimitive.Root bind:open>
	<DialogPrimitive.Portal>
		<DialogPrimitive.Overlay
			class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
		/>
		<DialogPrimitive.Content
			class="fixed left-1/2 top-[15%] z-50 flex max-h-[70vh] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 flex-col overflow-hidden rounded-xl border border-border bg-popover text-popover-foreground shadow-xl outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
		>
			<DialogPrimitive.Title class="shrink-0 px-4 pt-4 text-sm font-semibold">
				{$t('page.task.addRelation')}
			</DialogPrimitive.Title>
			<DialogPrimitive.Description class="sr-only">
				{$t('page.task.addRelationDescription')}
			</DialogPrimitive.Description>

			<div class="shrink-0 px-4 pt-3">
				<div class="flex flex-wrap gap-1.5">
					{#each CHOICES as c (c.key)}
						<button
							type="button"
							onclick={() => (choiceKey = c.key)}
							aria-pressed={choiceKey === c.key}
							class="rounded-full border px-2.5 py-1 text-xs transition-colors"
							class:border-primary={choiceKey === c.key}
							class:bg-primary={choiceKey === c.key}
							class:text-primary-foreground={choiceKey === c.key}
							class:border-border={choiceKey !== c.key}
							class:text-muted-foreground={choiceKey !== c.key}
						>
							{$t(`page.task.relation_${c.key}`)}
						</button>
					{/each}
				</div>
			</div>

			<div class="mt-3 flex items-center gap-2 border-b border-border px-4 py-2">
				<MagnifyingGlassIcon class="size-3.5 text-muted-foreground" />
				<!-- svelte-ignore a11y_autofocus -->
				<input
					value={query}
					oninput={onInput}
					type="text"
					placeholder={$t('page.task.relationSearchPlaceholder')}
					class="h-6 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
					autofocus
				/>
			</div>

			<div class="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-2">
				{#if searchError}
					<div class="px-2 py-6 text-center text-xs text-destructive">{searchError}</div>
				{:else if searching}
					<div class="px-2 py-6 text-center text-xs text-muted-foreground">
						{$t('page.search.searching')}
					</div>
				{:else if !isSearchable(query.trim())}
					<div class="px-2 py-6 text-center text-xs text-muted-foreground">
						{$t('page.task.relationSearchHint')}
					</div>
				{:else if selectable.length === 0}
					<div class="px-2 py-6 text-center text-xs text-muted-foreground">
						{$t('page.search.noTasksMatch')}
					</div>
				{:else}
					{#each selectable as task (task.id)}
						<button
							type="button"
							onclick={() => void pick(task)}
							class="flex items-start gap-2 rounded px-2 py-2 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
						>
							{#if task.status === 'completed'}
								<CheckCircleIcon
									class="mt-0.5 size-3.5 shrink-0 text-muted-foreground"
									weight="fill"
								/>
							{/if}
							<span class="min-w-0 flex-1">
								<span class="block truncate">{task.title}</span>
								<span class="block text-[10px] text-muted-foreground">#{task.id}</span>
							</span>
						</button>
					{/each}
				{/if}
			</div>
		</DialogPrimitive.Content>
	</DialogPrimitive.Portal>
</DialogPrimitive.Root>
