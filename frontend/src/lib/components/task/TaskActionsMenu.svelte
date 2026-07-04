<script lang="ts">
	import { getContext } from 'svelte';
	import { t } from '$lib/i18n';
	import { IsMobile } from '$lib/hooks';
	import type { DayPart, Priority, Task, TaskTemplate, TaskTemplateInput } from '$lib/api/types';
	import { PROJECT_SECTIONS_KEY, type ProjectSectionsCtx } from '$lib/context/projectSections';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { getApiClient } from '$lib/api/client';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { templates as templatesApi } from '$lib/api/endpoints/templates';
	import { templatesStore } from '$lib/stores/templates.svelte';
	import TemplateEditorDialog from '$lib/components/settings/TemplateEditorDialog.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { harpoonStore } from '$lib/stores/harpoon.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { taskSelectionStore } from '$lib/stores/taskSelection.svelte';
	import { toast } from 'svelte-sonner';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import LockSimpleOpenIcon from 'phosphor-svelte/lib/LockSimpleOpen';
	import GaugeIcon from 'phosphor-svelte/lib/Gauge';
	import { dayKeyInTz, dayStartUtcInTz, shiftDayKey, toIsoUtc } from '$lib/utils/format';
	import { nowStore } from '$lib/stores/now.svelte';
	import { PRIORITY_COLOR, PRIORITY_LABEL, PRIORITY_ORDER } from '$lib/utils/priority';
	import {
		applyParentLabelsToSubtasks,
		copyTaskTitle,
		deleteTask,
		describeError,
		duplicateTask,
		moveToBacklog,
		removeFromBacklog,
		togglePin,
		updateTaskFields,
		type ListMutator
	} from '$lib/utils/taskActions';
	import MoveTaskDialog from '$lib/components/dialog/MoveTaskDialog.svelte';
	import MoveSectionDialog from '$lib/components/dialog/MoveSectionDialog.svelte';
	import DecomposeTaskDialog from '$lib/components/dialog/DecomposeTaskDialog.svelte';
	import ArchiveIcon from 'phosphor-svelte/lib/Archive';
	import CheckSquareIcon from 'phosphor-svelte/lib/CheckSquare';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import ListIcon from 'phosphor-svelte/lib/List';
	import CalendarBlankIcon from 'phosphor-svelte/lib/CalendarBlank';
	import CopyIcon from 'phosphor-svelte/lib/Copy';
	import CopySimpleIcon from 'phosphor-svelte/lib/CopySimple';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import FlagIcon from 'phosphor-svelte/lib/Flag';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import ListBulletsIcon from 'phosphor-svelte/lib/ListBullets';
	import StackIcon from 'phosphor-svelte/lib/Stack';
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import AnchorIcon from 'phosphor-svelte/lib/Anchor';
	import PushPinIcon from 'phosphor-svelte/lib/PushPin';
	import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import XIcon from 'phosphor-svelte/lib/X';
	import type { Component } from 'svelte';

	let {
		task,
		mutator,
		belongs,
		hasSubtasks = false,
		selectIncludesSelf = true
	}: {
		task: Task;
		mutator: ListMutator;
		belongs?: (task: Task) => boolean;
		hasSubtasks?: boolean;
		selectIncludesSelf?: boolean;
	} = $props();

	const inInbox = $derived(task.inboxId !== null);

	const parentProject = $derived(
		task.projectId !== null ? projectsStore.items.find((p) => p.id === task.projectId) : null
	);
	const priorityLocked = $derived(settingsStore.troikiEnabled && !!parentProject?.troikiCategory);

	const tz = $derived(configStore.value?.timezone ?? null);
	const todayKey = $derived(nowStore.todayKey);
	const dueKey = $derived(task.dueAt ? dayKeyInTz(new Date(task.dueAt), tz) : null);

	const isToday = $derived(dueKey === todayKey);
	const isTomorrow = $derived(dueKey === shiftDayKey(todayKey, 1));
	const inBacklog = $derived(task.planState === 'backlog');

	// Setting a due date is a one-shot intent: close the menu after picking it.
	// Day part / priority are quick toggles users may chain, so those keep the
	// menu open (see setDayPart / setPriority below).
	function setDate(dayKey: string | null) {
		if (dayKey === null) {
			void updateTaskFields(task, mutator, { dueAt: null, dueHasTime: false }, { belongs });
			menuOpen = false;
			return;
		}
		const dueAt = toIsoUtc(dayStartUtcInTz(dayKey, tz));
		void updateTaskFields(task, mutator, { dueAt, dueHasTime: false }, { belongs });
		menuOpen = false;
	}

	function setDayPart(part: DayPart) {
		if (task.dayPart === part) return;
		void updateTaskFields(task, mutator, { dayPart: part }, { belongs });
	}

	function setPriority(p: Priority) {
		if (task.priority === p) return;
		void updateTaskFields(task, mutator, { priority: p }, { belongs });
	}

	const DAY_PARTS: Array<{ part: DayPart; labelKey: string; icon: Component }> = [
		{ part: 'morning', labelKey: 'task.dayPart.morning', icon: SunHorizonIcon as unknown as Component },
		{ part: 'afternoon', labelKey: 'task.dayPart.afternoon', icon: SunIcon as unknown as Component },
		{ part: 'evening', labelKey: 'task.dayPart.evening', icon: MoonIcon as unknown as Component }
	];

	const projectSectionsCtx = getContext<ProjectSectionsCtx | undefined>(PROJECT_SECTIONS_KEY);
	const showMoveToSection = $derived(
		(projectSectionsCtx?.sections.length ?? 0) > 0 && task.projectId !== null
	);

	const isHarpooned = $derived(harpoonStore.isHarpooned('task', task.id));

	async function toggleHarpoon() {
		try {
			if (isHarpooned) await harpoonStore.detach('task', task.id);
			else await harpoonStore.attach('task', task.id);
		} catch (err) {
			toast.error(describeError(err, $t('harpoon.toastFailed')));
		}
	}

	// On narrow screens a side-opening submenu can't fit next to the main menu
	// (both are ~14rem wide), so we inline the "Actions" items instead of nesting them.
	const isMobile = new IsMobile();

	let menuOpen = $state(false);
	let moveOpen = $state(false);
	let moveSectionOpen = $state(false);
	let decomposeOpen = $state(false);
	let templateEditorOpen = $state(false);
	let templateDraft = $state<TaskTemplate | null>(null);

	// Capture the task (and its flattened subtree) as a template draft, then open
	// the editor prefilled so the user can rename/adjust before saving.
	async function createTemplateFromTask(): Promise<void> {
		try {
			templateDraft = await tasksApi.templateDraft(getApiClient(), task.id);
			templateEditorOpen = true;
		} catch (err) {
			toast.error(describeError(err, $t('task.actions.createTemplateFailed')));
		}
	}

	async function saveTemplate(input: TaskTemplateInput): Promise<void> {
		try {
			const saved = await templatesApi.create(getApiClient(), input);
			templatesStore.upsert(saved);
			toast.success($t('task.actions.createTemplateSuccess'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.templates.toastSaveFailed')));
			throw err;
		}
	}
</script>

{#snippet actionItems()}
	<DropdownMenu.Item
		onclick={() => {
			if (!taskSelectionStore.mode) taskSelectionStore.enable();
			if (!selectIncludesSelf) return;
			if (taskSelectionStore.has(task.id)) taskSelectionStore.toggle(task.id);
			else taskSelectionStore.add(task.id);
		}}
	>
		<CheckSquareIcon class="size-4" />
		{selectIncludesSelf && taskSelectionStore.has(task.id)
			? $t('task.actions.deselect')
			: $t('task.actions.select')}
	</DropdownMenu.Item>
	{#if !inInbox}
		<DropdownMenu.Item
			onclick={async () => {
				const next = !task.isPrivate;
				await updateTaskFields(task, mutator, { isPrivate: next }, { belongs });
				toast.success($t('common.privacyUpdated'));
			}}
		>
			{#if task.isPrivate}
				<LockSimpleOpenIcon class="size-4" /> {$t('common.unmarkPrivate')}
			{:else}
				<LockSimpleIcon class="size-4" /> {$t('common.markPrivate')}
			{/if}
		</DropdownMenu.Item>
	{/if}
	<DropdownMenu.Item
		onclick={async () => {
			const next = !task.isComplex;
			await updateTaskFields(task, mutator, { isComplex: next }, { belongs });
			toast.success($t('task.actions.complexityUpdated'));
		}}
	>
		<GaugeIcon class="size-4" weight={task.isComplex ? 'fill' : 'regular'} />
		{task.isComplex ? $t('task.actions.unmarkComplex') : $t('task.actions.markComplex')}
	</DropdownMenu.Item>
	<DropdownMenu.Item onclick={() => (moveOpen = true)}>
		<FolderIcon class="size-4" /> {$t('task.actions.moveToProject')}
	</DropdownMenu.Item>
	{#if showMoveToSection}
		<DropdownMenu.Item onclick={() => (moveSectionOpen = true)}>
			<ListIcon class="size-4" /> {$t('task.actions.moveToSection')}
		</DropdownMenu.Item>
	{/if}
	{#if !inInbox}
		<DropdownMenu.Item
			disabled={hasSubtasks}
			title={hasSubtasks ? $t('task.actions.decomposeDisabled') : undefined}
			onclick={() => {
				if (!hasSubtasks) decomposeOpen = true;
			}}
		>
			<ListBulletsIcon class="size-4" /> {$t('task.actions.decompose')}
		</DropdownMenu.Item>
	{/if}
	<DropdownMenu.Item onclick={() => void toggleHarpoon()}>
		<AnchorIcon class="size-4" />
		{isHarpooned ? $t('harpoon.detach') : $t('harpoon.attach')}
	</DropdownMenu.Item>
	<DropdownMenu.Item onclick={() => void createTemplateFromTask()}>
		<StackIcon class="size-4" /> {$t('task.actions.createTemplate')}
	</DropdownMenu.Item>
	{#if hasSubtasks}
		<DropdownMenu.Item onclick={() => void applyParentLabelsToSubtasks(task, mutator)}>
			<TagIcon class="size-4" /> {$t('task.actions.applyLabelsToSubtasks')}
		</DropdownMenu.Item>
	{/if}
{/snippet}

<DropdownMenu.Root bind:open={menuOpen}>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			<Button
				{...props}
				size="sm"
				variant="ghost"
				class="size-8 p-0 text-muted-foreground hover:text-foreground"
				aria-label={$t('task.actions.ariaLabel')}
				onclick={(e: MouseEvent) => {
					e.stopPropagation();
					(props as { onclick?: (e: MouseEvent) => void }).onclick?.(e);
				}}
			>
				<DotsThreeIcon class="size-5" weight="bold" />
			</Button>
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Content align="end" class="min-w-[15rem]">
		<DropdownMenu.Item onclick={() => void copyTaskTitle(task)}>
			<CopyIcon class="size-4" /> {$t('task.actions.copy')}
		</DropdownMenu.Item>
		<DropdownMenu.Item onclick={() => void duplicateTask(task, mutator)}>
			<CopySimpleIcon class="size-4" /> {$t('task.actions.duplicate')}
		</DropdownMenu.Item>
		{#if !inInbox}
			<DropdownMenu.Item onclick={() => void togglePin(task, mutator)}>
				<PushPinIcon class="size-4" weight={task.isPinned ? 'fill' : 'regular'} />
				{task.isPinned ? $t('task.actions.unpin') : $t('task.actions.pin')}
			</DropdownMenu.Item>
		{/if}

		{#if isMobile.current}
			{@render actionItems()}
		{:else}
			<DropdownMenu.Sub>
				<DropdownMenu.SubTrigger>
					<DotsThreeIcon class="size-4" /> {$t('task.actions.moreSubmenu')}
				</DropdownMenu.SubTrigger>
				<DropdownMenu.SubContent class="min-w-[14rem]">
					{@render actionItems()}
				</DropdownMenu.SubContent>
			</DropdownMenu.Sub>
		{/if}

		{#if !inInbox}
		<DropdownMenu.Separator />

		<div class="px-2 py-1.5">
			<div class="mb-1.5 text-xs font-medium text-muted-foreground">{$t('task.actions.dateLabel')}</div>
			<div class="flex items-center gap-1">
				<button
					type="button"
					title={$t('common.today')}
					aria-label={$t('common.today')}
					aria-pressed={isToday}
					onclick={() => setDate(todayKey)}
					class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
					class:bg-accent={isToday}
				>
					<CalendarBlankIcon class="size-4 text-emerald-500" />
				</button>
				<button
					type="button"
					title={$t('common.tomorrow')}
					aria-label={$t('common.tomorrow')}
					aria-pressed={isTomorrow}
					onclick={() => setDate(shiftDayKey(todayKey, 1))}
					class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
					class:bg-accent={isTomorrow}
				>
					<SunIcon class="size-4 text-amber-500" />
				</button>
				<button
					type="button"
					title={inBacklog ? $t('task.actions.removeFromBacklog') : $t('task.actions.toBacklog')}
					aria-label={inBacklog ? $t('task.actions.removeFromBacklog') : $t('task.actions.toBacklog')}
					aria-pressed={inBacklog}
					onclick={() => {
						if (inBacklog) void removeFromBacklog(task, mutator, { belongs });
						else void moveToBacklog(task, mutator, { belongs });
						menuOpen = false;
					}}
					class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
					class:bg-accent={inBacklog}
				>
					<ArchiveIcon class="size-4 text-violet-500" />
				</button>
				<button
					type="button"
					title={$t('task.actions.clearDate')}
					aria-label={$t('task.actions.clearDate')}
					disabled={task.dueAt === null}
					onclick={() => setDate(null)}
					class="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
				>
					<XIcon class="size-4" />
				</button>
			</div>
		</div>

		<div class="px-2 py-1.5">
			<div class="mb-1.5 text-xs font-medium text-muted-foreground">{$t('task.actions.dayPartLabel')}</div>
			<div class="flex items-center gap-1">
				{#each DAY_PARTS as opt (opt.part)}
					{@const Icon = opt.icon}
					{@const active = task.dayPart === opt.part}
					{@const label = $t(opt.labelKey)}
					<button
						type="button"
						title={label}
						aria-label={label}
						aria-pressed={active}
						onclick={() => setDayPart(opt.part)}
						class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
						class:bg-accent={active}
					>
						<Icon class="size-4" />
					</button>
				{/each}
				<button
					type="button"
					title={$t('task.actions.clearDayPart')}
					aria-label={$t('task.actions.clearDayPart')}
					disabled={task.dayPart === 'none'}
					onclick={() => setDayPart('none')}
					class="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
				>
					<XIcon class="size-4" />
				</button>
			</div>
		</div>

		<div class="px-2 py-1.5">
			<div class="mb-1.5 text-xs font-medium text-muted-foreground">{$t('task.actions.priorityLabel')}</div>
			<div class="flex items-center gap-1">
				{#each PRIORITY_ORDER as p (p)}
					{@const active = task.priority === p}
					<button
						type="button"
						title={priorityLocked
							? $t('task.actions.priorityLockedTooltip')
							: PRIORITY_LABEL[p]}
						aria-label={PRIORITY_LABEL[p]}
						aria-pressed={active}
						disabled={priorityLocked}
						onclick={() => setPriority(p)}
						class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent"
						class:bg-accent={active}
					>
						<FlagIcon
							class={`size-4 ${PRIORITY_COLOR[p]}`}
							weight={p === 'no-priority' ? 'regular' : 'fill'}
						/>
					</button>
				{/each}
			</div>
			{#if priorityLocked}
				<div class="mt-1 text-[10px] text-muted-foreground">
					{$t('task.actions.lockedByTroiki')}
				</div>
			{/if}
		</div>
		{/if}

		<DropdownMenu.Separator />

		<DropdownMenu.Item variant="destructive" onclick={() => void deleteTask(task, mutator)}>
			<TrashIcon class="size-4" /> {$t('common.delete')}
		</DropdownMenu.Item>
	</DropdownMenu.Content>
</DropdownMenu.Root>

<MoveTaskDialog bind:open={moveOpen} {task} {mutator} {belongs} />
<MoveSectionDialog bind:open={moveSectionOpen} {task} {mutator} {belongs} sections={projectSectionsCtx?.sections} />
<DecomposeTaskDialog bind:open={decomposeOpen} {task} {mutator} />
<TemplateEditorDialog bind:open={templateEditorOpen} prefill={templateDraft} onSave={saveTemplate} />
