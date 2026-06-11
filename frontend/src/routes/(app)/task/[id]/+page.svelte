<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeft';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import XIcon from 'phosphor-svelte/lib/X';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import TextAlignStartIcon from 'phosphor-svelte/lib/TextAlignLeft';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { Popover as PopoverPrimitive } from 'bits-ui';
	import { parseDate, type DateValue } from '@internationalized/date';
	import { Button } from '$lib/components/ui/button';
	import { Calendar } from '$lib/components/ui/calendar';
	import { getApiClient } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { configStore } from '$lib/stores/config.svelte';
	import { appSettingsStore } from '$lib/stores/appSettings.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { viewFilterStore } from '$lib/stores/viewFilter.svelte';
	import type { DayPart, Priority, Task, TaskInput } from '$lib/api/types';
	import type { ListMutator } from '$lib/utils/taskActions';
	import PriorityPicker from '$lib/components/task/PriorityPicker.svelte';
	import DayPartPicker from '$lib/components/task/DayPartPicker.svelte';
	import RecurrencePicker from '$lib/components/task/RecurrencePicker.svelte';
	import TaskActionsMenu from '$lib/components/task/TaskActionsMenu.svelte';
	import MoveTaskDialog from '$lib/components/dialog/MoveTaskDialog.svelte';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import CompletedTasksGroup from '$lib/components/project/CompletedTasksGroup.svelte';
	import { comparePriority } from '$lib/utils/priority';
	import { dayKeyInTz, dayStartUtcInTz, parseIso, shiftDayKey, toIsoUtc, weekRangeKeys } from '$lib/utils/format';
	import { nowStore } from '$lib/stores/now.svelte';
	import { describeError, toggleComplete } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { useInvalidation } from '$lib/hooks/useInvalidation.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { currentTaskStore } from '$lib/stores/currentTask.svelte';
	import { t, locale } from '$lib/i18n';
	import MarkdownText from '$lib/components/MarkdownText.svelte';
	import MarkdownRich from '$lib/components/MarkdownRich.svelte';
	import TroikiTriggerIcon from '$lib/components/app/TroikiTriggerIcon.svelte';
	import { hasMarkdownContent, hasMarkdownLink } from '$lib/utils/markdown';
	import { tick, untrack } from 'svelte';

	const taskId = $derived(Number(page.params.id));

	let task = $state<Task | null>(null);
	let notFound = $state(false);
	let saving = $state(false);

	const subtasks = useListMutator<Task>();
	let newSubtaskTitle = $state('');
	let creatingSubtask = $state(false);
	let subtaskInputEl = $state<HTMLInputElement | undefined>();

	const sortedSubtasks = $derived(
		[...subtasks.items].sort((a, b) => comparePriority(a.priority, b.priority))
	);
	const openSubtasks = $derived(sortedSubtasks.filter((t) => t.status !== 'completed'));
	const completedSubtasks = $derived(sortedSubtasks.filter((t) => t.status === 'completed'));

	let title = $state('');
	let description = $state('');
	let descriptionFocused = $state(false);
	let titleFocused = $state(false);
	let descriptionEl = $state<HTMLTextAreaElement | undefined>();
	let titleEl = $state<HTMLTextAreaElement | undefined>();

	const titleHasLink = $derived(hasMarkdownLink(title));
	const descriptionHasMarkdown = $derived(hasMarkdownContent(description));
	const showTitleRendered = $derived(titleHasLink && !titleFocused);
	const showDescriptionRendered = $derived(descriptionHasMarkdown && !descriptionFocused);

	async function focusTitle(): Promise<void> {
		titleFocused = true;
		await tick();
		titleEl?.focus();
		const len = title.length;
		titleEl?.setSelectionRange(len, len);
		autoGrow(titleEl);
	}

	async function focusDescription(): Promise<void> {
		descriptionFocused = true;
		await tick();
		descriptionEl?.focus();
		const len = description.length;
		descriptionEl?.setSelectionRange(len, len);
		autoGrow(descriptionEl);
	}

	function autoGrow(el: HTMLTextAreaElement | undefined): void {
		if (!el) return;
		el.style.height = 'auto';
		el.style.height = `${el.scrollHeight}px`;
	}

	$effect(() => {
		void description;
		autoGrow(descriptionEl);
	});

	const privateHiddenMsg = $derived($t('common.privateHidden'));
	$effect(() => {
		const cur = task;
		if (!cur || !settingsStore.publicView) return;
		const projectPrivate =
			cur.projectId !== null &&
			(projectsStore.items.find((p) => p.id === cur.projectId)?.isPrivate ?? false);
		if (cur.isPrivate || projectPrivate) {
			toast.info(privateHiddenMsg);
			void goto(resolve('/today'));
		}
	});

	$effect(() => {
		void title;
		autoGrow(titleEl);
	});
	let priority = $state<Priority>('no-priority');

	const checkboxClass = $derived.by(() => {
		const base = 'mt-1 inline-flex size-5 shrink-0 items-center justify-center rounded-full border-[1.5px] transition-colors focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50';
		const completed = task?.status === 'completed';
		if (completed) {
			if (priority === 'high') return `${base} border-red-500 bg-red-500 text-white`;
			if (priority === 'medium') return `${base} border-amber-500 bg-amber-500 text-white`;
			if (priority === 'low') return `${base} border-blue-500 bg-blue-500 text-white`;
			return `${base} border-zinc-500 bg-zinc-500 text-white dark:border-zinc-600 dark:bg-zinc-600`;
		}
		if (priority === 'high') return `${base} border-red-500`;
		if (priority === 'medium') return `${base} border-amber-500`;
		if (priority === 'low') return `${base} border-blue-500`;
		return `${base} border-border hover:border-primary`;
	});
	let dayPart = $state<DayPart>('none');
	let dueDate = $state('');
	let recurrence = $state<string | null>(null);
	let labelIds = $state<string[]>([]);
	let removedAuto = $state<string[]>([]);
	const projectTroikiCategory = $derived.by(() => {
		const t = task;
		if (!t || t.projectId === null) return null;
		const projectId = t.projectId;
		return projectsStore.items.find((p) => p.id === projectId)?.troikiCategory ?? null;
	});
	const projectTroikiLocked = $derived(
		settingsStore.troikiEnabled && projectTroikiCategory !== null
	);
	const inInbox = $derived(task?.inboxId !== null && task?.inboxId !== undefined);

	let moveDialogOpen = $state(false);
	let datePopoverOpen = $state(false);

	const calendarValue = $derived<DateValue | undefined>(
		dueDate ? parseDate(dueDate) : undefined
	);

	function pad(n: number): string {
		return n < 10 ? `0${n}` : String(n);
	}

	function setCalendarValue(v: DateValue | undefined): void {
		if (!v) {
			dueDate = '';
		} else {
			dueDate = `${v.year}-${pad(v.month)}-${pad(v.day)}`;
		}
		datePopoverOpen = false;
		scheduleSave();
	}

	const todayKey = $derived(nowStore.todayKey);
	const tomorrowKey = $derived(shiftDayKey(todayKey, 1));
	const isToday = $derived(dueDate === todayKey);
	const isTomorrow = $derived(dueDate === tomorrowKey);
	const isCustomDate = $derived(!!dueDate && !isToday && !isTomorrow);
	const weekRange = $derived(weekRangeKeys(nowStore.now, configStore.value?.timezone ?? null));
	const calendarLocale = $derived($locale === 'ru' ? 'ru-RU' : 'en-US');

	// Non-reactive flag — guards against auto-save during initial hydration
	let allowSave = false;
	let saveTimer: ReturnType<typeof setTimeout> | null = null;

	const allLabels = $derived([...labelsStore.favourites, ...labelsStore.rest]);
	const project = $derived(
		task?.projectId ? projectsStore.items.find((p) => p.id === task!.projectId) : null
	);
	const autoLabelNames = $derived.by(() => {
		const byId = new Map(allLabels.map((l) => [l.id, l.name]));
		const names: string[] = [];
		for (const rule of appSettingsStore.autoLabels) {
			for (const id of rule.labelIds) {
				const name = byId.get(id);
				if (name && !names.includes(name)) names.push(name);
			}
		}
		return new Set(names);
	});
	const selectedLabels = $derived(
		labelIds
			.map((id) => allLabels.find((l) => String(l.id) === id))
			.filter((l): l is (typeof allLabels)[number] => !!l)
	);

	function arrayEquals(a: readonly string[], b: readonly string[]): boolean {
		if (a.length !== b.length) return false;
		for (let i = 0; i < a.length; i++) {
			if (a[i] !== b[i]) return false;
		}
		return true;
	}

	function hydrate(t: Task): void {
		// Server returned the exact revision we already have (typical for the SSE
		// echo that follows our own save). Skip everything — assigning identical
		// values still flushes derived/effects and causes a visible flicker.
		if (task && task.id === t.id && task.updatedAt === t.updatedAt) return;

		allowSave = false;
		task = t;
		if (title !== t.title) title = t.title;
		const nextDescription = t.description ?? '';
		if (description !== nextDescription) description = nextDescription;
		if (priority !== t.priority) priority = t.priority;
		if (dayPart !== t.dayPart) dayPart = t.dayPart;
		const nextRecurrence = t.recurrenceRule ?? null;
		if (recurrence !== nextRecurrence) recurrence = nextRecurrence;
		const dt = parseIso(t.dueAt);
		const nextDueDate = dt ? dayKeyInTz(dt, configStore.value?.timezone ?? null) : '';
		if (dueDate !== nextDueDate) dueDate = nextDueDate;
		const nextLabelIds = t.labels.map((l) => String(l.id));
		if (!arrayEquals(labelIds, nextLabelIds)) labelIds = nextLabelIds;
		if (removedAuto.length > 0) removedAuto = [];
		// Permit auto-save only after all reactive effects from hydration have flushed
		setTimeout(() => {
			allowSave = true;
		}, 0);
	}

	function scheduleSave(): void {
		if (!allowSave || !task || !title.trim()) return;
		if (saveTimer !== null) clearTimeout(saveTimer);
		saveTimer = setTimeout(() => void save(), 800);
	}

	// Watch picker bindings for auto-save
	$effect(() => {
		void priority;
		scheduleSave();
	});
	$effect(() => {
		void dayPart;
		scheduleSave();
	});
	$effect(() => {
		void recurrence;
		scheduleSave();
	});
	$effect(() => {
		if (task) viewFilterStore.setTitle(task.title);
		return () => viewFilterStore.clear();
	});

	$effect(() => {
		if (task) currentTaskStore.set(task.projectId, task.labels.map((l) => l.id));
		return () => currentTaskStore.clear();
	});

	const loader = usePageLoad(
		async (isValid) => {
			notFound = false;
			if (task?.id !== taskId) {
				task = null;
				subtasks.items = [];
			}
			if (!Number.isFinite(taskId)) {
				notFound = true;
				return;
			}
			const client = getApiClient();
			const t = await tasksApi.get(client, taskId, { includeSubtasks: true });
			if (!isValid()) return;
			hydrate(t);
			subtasks.items = t.subtasks?.items ?? [];
		},
		{
			autoLoad: false,
			initialLoading: true,
			onError(err) {
				if (err instanceof ApiError && err.code === 'not_found') {
					notFound = true;
					return;
				}
				toast.error(describeError(err, $t('page.task.errorLoading')));
			}
		}
	);

	const pageMutator: ListMutator = {
		replace(updated: Task) {
			hydrate(updated);
		},
		remove(_id: number) {
			void goto(resolve('/inbox'));
		}
	};

	async function addSubtask(): Promise<void> {
		const trimmed = newSubtaskTitle.trim();
		if (!trimmed || !task || creatingSubtask) return;
		creatingSubtask = true;
		try {
			const input: TaskInput = { title: trimmed };
			if (projectTroikiLocked) {
				input.priority = task.priority;
			}
			const created = await tasksApi.createSubtask(getApiClient(), task.id, input);
			subtasks.items = [...subtasks.items, created];
			window.dispatchEvent(
				new CustomEvent('turboist:task-created', {
					detail: {
						task: created,
						projectId: created.projectId,
						contextId: created.contextId
					}
				})
			);
			newSubtaskTitle = '';
			const subs = await tasksApi
				.listSubtasks(getApiClient(), task.id)
				.catch(() => null);
			if (subs) subtasks.items = subs.items;
		} catch (err) {
			toast.error(describeError(err, $t('page.task.failedAddSubtask')));
		} finally {
			creatingSubtask = false;
			await tick();
			subtaskInputEl?.focus();
		}
	}

	function onSubtaskKeydown(e: KeyboardEvent): void {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			void addSubtask();
		}
	}

	function toggleLabel(id: string, name: string, isAuto: boolean): void {
		if (labelIds.includes(id)) {
			labelIds = labelIds.filter((x) => x !== id);
			if (isAuto && !removedAuto.includes(name)) removedAuto = [...removedAuto, name];
		} else {
			labelIds = [...labelIds, id];
			if (isAuto) removedAuto = removedAuto.filter((n) => n !== name);
		}
		scheduleSave();
	}

	function setDate(value: string): void {
		dueDate = dueDate === value ? '' : value;
		scheduleSave();
	}

async function save(): Promise<void> {
		if (!task || saving || !title.trim()) return;
		saving = true;
		try {
			const dueAt: string | null = dueDate
				? toIsoUtc(dayStartUtcInTz(dueDate, configStore.value?.timezone ?? null))
				: null;
			const payload: TaskInput = {
				title: title.trim(),
				description,
				priority,
				dayPart,
				dueAt,
				dueHasTime: false,
				recurrenceRule: recurrence,
				labels: labelIds
					.map((id) => allLabels.find((l) => String(l.id) === id)?.name)
					.filter((n): n is string => !!n),
				removedAutoLabels: removedAuto
			};
			const updated = await tasksApi.update(getApiClient(), task.id, payload);
			hydrate(updated);
		} catch (err) {
			toast.error(describeError(err, $t('page.task.failedSave')));
		} finally {
			saving = false;
		}
	}

	function back(): void {
		if (history.length > 1) history.back();
		else void goto(resolve('/inbox'));
	}

	$effect(() => {
		if (Number.isFinite(taskId)) untrack(() => void loader.refetch());
	});

	useInvalidation(['tasks'], () => void loader.revalidate());
</script>

<header class="flex items-center justify-between gap-3 border-b border-border px-2 py-1 sm:px-5">
	<div class="flex min-w-0 items-center gap-2">
		<Button variant="ghost" size="sm" onclick={back} class="h-7 shrink-0 gap-1 px-2 text-[10px] uppercase tracking-wider">
			<ArrowLeftIcon class="size-3" />
			{$t('common.back')}
		</Button>
		{#if project}
			<a
				href={resolve('/(app)/project/[id]', { id: String(project.id) })}
				class="inline-flex min-w-0 items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
			>
				<span
					class="size-2 shrink-0 rounded-full"
					style={`background-color: ${project.color}`}
				></span>
				<span class="truncate">{project.title}{#if settingsStore.troikiEnabled && project.troikiCategory}<TroikiTriggerIcon class="ml-1.5 inline-block size-3 align-middle text-muted-foreground/50" />{/if}</span>
			</a>
		{/if}
	</div>
	{#if task}
		<TaskActionsMenu task={task} mutator={pageMutator} selectIncludesSelf={false} />
	{/if}
</header>

{#if loader.loading}
	<div class="px-6 py-8 text-sm text-muted-foreground">{$t('app.loading')}</div>
{:else if loader.error && !notFound}
	<div class="px-6 py-8 text-sm text-muted-foreground">{loader.error}</div>
{:else if notFound || !task}
	<div class="px-6 py-8 text-sm text-muted-foreground">{$t('page.task.notFound')}</div>
{:else}
	<form
		onsubmit={(e) => {
			e.preventDefault();
		}}
		class="grid gap-8 p-6 sm:grid-cols-[1fr_16rem] sm:p-8"
	>
		<div class="flex min-w-0 flex-col gap-4">
			<div class="flex items-start gap-3">
				<button
					type="button"
					onclick={() => task && void toggleComplete(task, pageMutator, { removeWhenCompleted: false })}
					aria-label={task?.status === 'completed' ? $t('task.markIncomplete') : $t('task.markComplete')}
					class={checkboxClass}
				>
					{#if task?.status === 'completed'}
						<CheckIcon class="size-3" weight="bold" />
					{/if}
				</button>
				<div class="min-w-0 flex-1">
					{#if showTitleRendered}
						<div
							role="textbox"
							tabindex="0"
							aria-label={$t('common.title')}
							onclick={() => void focusTitle()}
							onkeydown={(e) => {
								if (e.key === 'Enter' || e.key === ' ') {
									e.preventDefault();
									void focusTitle();
								}
							}}
							class="block w-full cursor-text break-words text-xl font-semibold leading-tight outline-none {task?.status === 'completed' ? 'text-muted-foreground line-through' : ''}"
						>
							<MarkdownText text={title} linkClass="text-muted-foreground underline underline-offset-2 hover:text-foreground" />
						</div>
					{:else}
						<textarea
							bind:this={titleEl}
							bind:value={title}
							aria-label={$t('common.title')}
							placeholder={$t('page.task.namePlaceholder')}
							rows="1"
							oninput={(e) => {
								autoGrow(e.currentTarget as HTMLTextAreaElement);
								scheduleSave();
							}}
							onfocus={() => (titleFocused = true)}
							onblur={() => (titleFocused = false)}
							onkeydown={(e) => {
								if (e.key === 'Enter') {
									e.preventDefault();
									(e.currentTarget as HTMLTextAreaElement).blur();
								}
							}}
							class="block w-full resize-none overflow-hidden break-words bg-transparent text-xl font-semibold leading-tight outline-none placeholder:text-muted-foreground/60 {task?.status === 'completed' ? 'text-muted-foreground line-through' : ''}"
						></textarea>
					{/if}
				</div>
			</div>
			<div class="relative pl-2">
				{#if !description && !descriptionFocused}
					<TextAlignStartIcon class="pointer-events-none absolute left-2 top-[2px] size-3.5 text-muted-foreground/40" />
				{/if}
				{#if showDescriptionRendered}
					<div
						role="textbox"
						tabindex="0"
						aria-label={$t('common.description')}
						onclick={() => void focusDescription()}
						onkeydown={(e) => {
							if (e.key === 'Enter' || e.key === ' ') {
								e.preventDefault();
								void focusDescription();
							}
						}}
						class="block w-full cursor-text [overflow-wrap:anywhere] text-sm leading-relaxed text-foreground/70 outline-none"
					>
						<MarkdownRich text={description} />
					</div>
				{:else}
					<textarea
						bind:this={descriptionEl}
						bind:value={description}
						aria-label={$t('common.description')}
						placeholder={$t('page.task.descriptionPlaceholder')}
						rows="1"
						oninput={(e) => {
							autoGrow(e.currentTarget as HTMLTextAreaElement);
							scheduleSave();
						}}
						onfocus={() => (descriptionFocused = true)}
						onblur={() => (descriptionFocused = false)}
						class="block w-full resize-none overflow-hidden bg-transparent text-sm leading-relaxed text-foreground/70 outline-none placeholder:text-muted-foreground/60"
						class:pl-5={!description && !descriptionFocused}
					></textarea>
				{/if}
			</div>

			<section class="flex flex-col gap-2">
				<div class="flex items-baseline justify-between gap-2">
					<span class="ml-2 text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
						{$t('page.task.subtasks')}
					</span>
					{#if !inInbox && subtasks.items.length > 0}
						<span class="mr-2 text-[11px] text-muted-foreground/70">{subtasks.items.length}</span>
					{/if}
				</div>
				{#if inInbox}
					<div class="rounded-md border border-border/60 bg-muted/40 px-3 py-2.5 text-sm text-muted-foreground">
						{$t('page.task.inboxSubtasksNoticePre')}
						<button
							type="button"
							onclick={() => (moveDialogOpen = true)}
							class="underline underline-offset-2 transition-colors hover:text-foreground"
						>{$t('page.task.inboxSubtasksNoticeLink')}</button>.
					</div>
				{:else}
					{#if subtasks.items.length > 0}
						<div class="overflow-hidden rounded-md border border-border/60">
							{#if openSubtasks.length > 0}
								<TaskTree
									tasks={openSubtasks}
									showProject={false}
									mutator={subtasks.mutator}
									onToggle={(t) =>
										toggleComplete(t, subtasks.mutator, { removeWhenCompleted: false })}
								/>
							{/if}
							{#if completedSubtasks.length > 0}
								<CompletedTasksGroup
									tasks={completedSubtasks}
									draggable={false}
									mutator={subtasks.mutator}
									onToggle={(t) =>
										toggleComplete(t, subtasks.mutator, { removeWhenCompleted: false })}
								/>
							{/if}
						</div>
					{/if}
					<div class="flex items-center gap-2 rounded-md border border-dashed border-border/70 bg-muted/20 px-2.5 py-1.5 transition-colors focus-within:border-border focus-within:bg-muted/40">
						<PlusIcon class="size-3.5 shrink-0 text-muted-foreground" />
						<input
							bind:this={subtaskInputEl}
							bind:value={newSubtaskTitle}
							onkeydown={onSubtaskKeydown}
							disabled={creatingSubtask}
							placeholder={$t('page.task.addSubtaskPlaceholder')}
							aria-label={$t('page.task.addSubtaskPlaceholder')}
							class="w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground/60 disabled:opacity-60"
						/>
						{#if newSubtaskTitle.trim()}
							<Button
								type="button"
								size="xs"
								variant="secondary"
								onclick={() => void addSubtask()}
								disabled={creatingSubtask}
							>
								{$t('common.add')}
							</Button>
						{/if}
					</div>
				{/if}
			</section>
		</div>

		<aside class="flex flex-col gap-5 sm:border-l sm:border-border sm:pl-6">
			{#if !inInbox}
			<div class="flex flex-col gap-1.5">
				<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
					{$t('page.task.date')}
				</span>
				<div class="flex items-center gap-1.5">
				<div
					class="inline-flex w-fit items-center gap-0.5 rounded-md border border-border bg-background p-0.5"
					role="group"
					aria-label={$t('dialog.quickAdd.dueDateAriaLabel')}
				>
					<button
						type="button"
						onclick={() => setDate(todayKey)}
						aria-pressed={isToday}
						class="inline-flex h-7 items-center rounded-[5px] px-2.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50"
						class:bg-accent={isToday}
						class:text-foreground={isToday}
						class:text-muted-foreground={!isToday}
						class:hover:bg-accent={!isToday}
						class:hover:text-foreground={!isToday}
					>
						{$t('common.today')}
					</button>
					<button
						type="button"
						onclick={() => setDate(tomorrowKey)}
						aria-pressed={isTomorrow}
						class="inline-flex h-7 items-center rounded-[5px] px-2.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50"
						class:bg-accent={isTomorrow}
						class:text-foreground={isTomorrow}
						class:text-muted-foreground={!isTomorrow}
						class:hover:bg-accent={!isTomorrow}
						class:hover:text-foreground={!isTomorrow}
					>
						{$t('common.tomorrow')}
					</button>
					<PopoverPrimitive.Root bind:open={datePopoverOpen}>
						<PopoverPrimitive.Trigger
							aria-pressed={isCustomDate}
							aria-label={$t('dialog.quickAdd.customDateAriaLabel')}
							title={isCustomDate ? dueDate : $t('dialog.quickAdd.pickDateTitle')}
							class="relative inline-flex h-7 items-center gap-1 rounded-[5px] px-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50 {isCustomDate
								? 'bg-accent text-foreground'
								: 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
						>
							{#if isCustomDate}
								<span class="font-mono text-[11px]">{dueDate}</span>
							{:else}
								<DotsThreeIcon class="size-4" weight="bold" />
							{/if}
						</PopoverPrimitive.Trigger>
						<PopoverPrimitive.Portal>
							<PopoverPrimitive.Content
								align="end"
								sideOffset={6}
								class="z-50 rounded-md border border-border bg-popover text-popover-foreground shadow-md outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
							>
								<Calendar
									type="single"
									value={calendarValue}
									onValueChange={setCalendarValue}
									captionLayout="dropdown"
									locale={calendarLocale}
									highlightWeek={weekRange}
								/>
							</PopoverPrimitive.Content>
						</PopoverPrimitive.Portal>
					</PopoverPrimitive.Root>
				</div>
				{#if dueDate}
					<button
						type="button"
						onclick={() => { dueDate = ''; scheduleSave(); }}
						aria-label={$t('page.task.clearDate')}
						title={$t('page.task.clearDate')}
						class="inline-flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
					>
						<XIcon class="size-3.5" />
					</button>
				{/if}
				</div>
			</div>

			<div class="grid grid-cols-2 gap-5 sm:contents">
				<div class="flex flex-col gap-1.5">
					<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
						{$t('page.task.priority')}
					</span>
					<PriorityPicker bind:value={priority} disabled={projectTroikiLocked} />
					{#if projectTroikiLocked}
						<span class="text-[10px] text-muted-foreground">
							{$t('page.task.priorityLockedByTroiki')}
						</span>
					{/if}
				</div>

				<div class="flex flex-col gap-1.5">
					<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
						{$t('page.task.dayPart')}
					</span>
					<DayPartPicker bind:value={dayPart} />
				</div>
			</div>
			{/if}

			<div class="flex flex-col gap-1.5">
				<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
					{$t('page.task.repeat')}
				</span>
				<RecurrencePicker bind:value={recurrence} />
			</div>

			{#if allLabels.length > 0}
				<div class="flex flex-col gap-1.5">
					<span
						class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground"
					>
						{$t('page.task.labels')}
					</span>
					{#if selectedLabels.length > 0}
						<div class="flex flex-wrap gap-1">
							{#each selectedLabels as label (label.id)}
								<button
									type="button"
									onclick={() =>
										toggleLabel(String(label.id), label.name, autoLabelNames.has(label.name))}
									class="group/chip inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium text-foreground transition-opacity hover:opacity-80"
									style={label.color
										? `background-color: color-mix(in srgb, ${label.color} 32%, transparent);`
										: 'background-color: var(--accent);'}
								>
									{label.name}
									<XIcon class="size-3 opacity-60 transition-opacity group-hover/chip:opacity-100" />
								</button>
							{/each}
						</div>
					{/if}
					<details class="group/labels">
						<summary
							class="inline-flex cursor-pointer list-none items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
						>
							<span>{$t('page.task.editLabels')}</span>
						</summary>
						<div class="mt-2 flex flex-wrap gap-1">
							{#each allLabels as label (label.id)}
								{@const id = String(label.id)}
								{@const active = labelIds.includes(id)}
								<button
									type="button"
									onclick={() => toggleLabel(id, label.name, autoLabelNames.has(label.name))}
									class="rounded-md px-2 py-0.5 text-xs transition-colors"
									class:font-medium={active}
									class:text-foreground={active}
									class:border={!active}
									class:border-border={!active}
									class:text-muted-foreground={!active}
									class:hover:bg-accent={!active}
									style={active && label.color
										? `background-color: color-mix(in srgb, ${label.color} 32%, transparent);`
										: undefined}
								>
									{label.name}
								</button>
							{/each}
						</div>
					</details>
				</div>
			{/if}
		</aside>
	</form>
{/if}

<MoveTaskDialog bind:open={moveDialogOpen} task={task} mutator={pageMutator} />
