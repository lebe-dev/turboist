<script lang="ts">
	import { getContext, tick } from 'svelte';
	import { t } from '$lib/i18n';
	import { IsMobile } from '$lib/hooks';
	import type { DayPart, Priority, Task, TaskTemplate, TaskTemplateInput } from '$lib/api/types';
	import { PROJECT_SECTIONS_KEY, type ProjectSectionsCtx } from '$lib/context/projectSections';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import * as Sheet from '$lib/components/ui/sheet';
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
		copyTaskId,
		copyTaskJson,
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
	import HashIcon from 'phosphor-svelte/lib/Hash';
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
	import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeft';
	import CaretRightIcon from 'phosphor-svelte/lib/CaretRight';
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
		{
			part: 'morning',
			labelKey: 'task.dayPart.morning',
			icon: SunHorizonIcon as unknown as Component
		},
		{
			part: 'afternoon',
			labelKey: 'task.dayPart.afternoon',
			icon: SunIcon as unknown as Component
		},
		{
			part: 'evening',
			labelKey: 'task.dayPart.evening',
			icon: MoonIcon as unknown as Component
		}
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

	const isMobile = new IsMobile();

	let menuOpen = $state(false);
	let mobilePage = $state<'main' | 'more'>('main');
	let mobileContent = $state<HTMLDivElement>();
	let moreButton = $state<HTMLButtonElement | null>(null);
	let backButton = $state<HTMLButtonElement | null>(null);
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

	interface MenuAction {
		id: string;
		label: string;
		icon: typeof CopyIcon;
		run: () => void | Promise<void>;
		rare?: boolean;
		disabled?: boolean;
		title?: string;
		weight?: 'fill' | 'regular';
	}

	const primaryActions: MenuAction[] = $derived([
		{
			id: 'copy',
			label: $t('task.actions.copy'),
			icon: CopyIcon,
			run: () => copyTaskTitle(task)
		},
		{
			id: 'json',
			label: $t('task.actions.copyJson'),
			icon: CopyIcon,
			run: () => copyTaskJson(task),
			rare: true
		},
		{
			id: 'id',
			label: $t('task.actions.copyId'),
			icon: HashIcon,
			run: () => copyTaskId(task),
			rare: true
		},
		{
			id: 'duplicate',
			label: $t('task.actions.duplicate'),
			icon: CopySimpleIcon,
			run: () => duplicateTask(task, mutator)
		},
		...(!inInbox
			? [
					{
						id: 'pin',
						label: task.isPinned ? $t('task.actions.unpin') : $t('task.actions.pin'),
						icon: PushPinIcon,
						run: () => togglePin(task, mutator),
						rare: true,
						weight: task.isPinned ? ('fill' as const) : ('regular' as const)
					}
				]
			: [])
	]);

	const extraActions: MenuAction[] = $derived([
		{
			id: 'select',
			icon: CheckSquareIcon,
			rare: true,
			label:
				selectIncludesSelf && taskSelectionStore.has(task.id)
					? $t('task.actions.deselect')
					: $t('task.actions.select'),
			run: () => {
				if (!taskSelectionStore.mode) taskSelectionStore.enable();
				if (!selectIncludesSelf) return;
				if (taskSelectionStore.has(task.id)) taskSelectionStore.toggle(task.id);
				else taskSelectionStore.add(task.id);
			}
		},
		...(!inInbox
			? [
					{
						id: 'privacy',
						rare: true,
						label: task.isPrivate ? $t('common.unmarkPrivate') : $t('common.markPrivate'),
						icon: task.isPrivate ? LockSimpleOpenIcon : LockSimpleIcon,
						run: async () => {
							await updateTaskFields(task, mutator, { isPrivate: !task.isPrivate }, { belongs });
							toast.success($t('common.privacyUpdated'));
						}
					}
				]
			: []),
		{
			id: 'complexity',
			icon: GaugeIcon,
			label: task.isComplex ? $t('task.actions.unmarkComplex') : $t('task.actions.markComplex'),
			weight: task.isComplex ? 'fill' : 'regular',
			run: async () => {
				await updateTaskFields(task, mutator, { isComplex: !task.isComplex }, { belongs });
				toast.success($t('task.actions.complexityUpdated'));
			}
		},
		{
			id: 'project',
			label: $t('task.actions.moveToProject'),
			icon: FolderIcon,
			rare: true,
			run: () => {
				moveOpen = true;
			}
		},
		...(showMoveToSection
			? [
					{
						id: 'section',
						label: $t('task.actions.moveToSection'),
						icon: ListIcon,
						rare: true,
						run: () => {
							moveSectionOpen = true;
						}
					}
				]
			: []),
		...(!inInbox
			? [
					{
						id: 'decompose',
						label: $t('task.actions.decompose'),
						icon: ListBulletsIcon,
						rare: true,
						disabled: hasSubtasks,
						title: hasSubtasks ? $t('task.actions.decomposeDisabled') : undefined,
						run: () => {
							if (!hasSubtasks) decomposeOpen = true;
						}
					}
				]
			: []),
		{
			id: 'harpoon',
			label: isHarpooned ? $t('harpoon.detach') : $t('harpoon.attach'),
			icon: AnchorIcon,
			run: toggleHarpoon
		},
		{
			id: 'template',
			label: $t('task.actions.createTemplate'),
			icon: StackIcon,
			rare: true,
			run: createTemplateFromTask
		},
		...(hasSubtasks
			? [
					{
						id: 'labels',
						label: $t('task.actions.applyLabelsToSubtasks'),
						icon: TagIcon,
						rare: true,
						run: () => applyParentLabelsToSubtasks(task, mutator)
					}
				]
			: [])
	]);

	const mobileActions = $derived([...primaryActions, ...extraActions]);

	async function showMobilePage(page: 'main' | 'more') {
		mobilePage = page;
		await tick();
		if (mobileContent) mobileContent.scrollTop = 0;
		(page === 'more' ? backButton : moreButton)?.focus({ preventScroll: true });
	}

	function runMobileAction(action: MenuAction) {
		menuOpen = false;
		void action.run();
	}
</script>

{#snippet actionItems(actions: MenuAction[])}
	{#each actions as action (action.id)}
		{@const Icon = action.icon}
		{#if isMobile.current}
			<button
				type="button"
				class="flex min-h-11 w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm hover:bg-accent focus-visible:outline-ring disabled:opacity-50 [&_svg]:shrink-0"
				disabled={action.disabled}
				title={action.title}
				onclick={() => runMobileAction(action)}
			>
				<Icon class="size-4" weight={action.weight ?? 'regular'} />
				{action.label}
			</button>
		{:else}
			<DropdownMenu.Item
				disabled={action.disabled}
				title={action.title}
				onclick={() => void action.run()}
			>
				<Icon class="size-4" weight={action.weight ?? 'regular'} />
				{action.label}
			</DropdownMenu.Item>
		{/if}
	{/each}
{/snippet}

{#snippet planningControls()}
	<div class="px-2 py-1.5">
		<div class="mb-1.5 text-xs font-medium text-muted-foreground">
			{$t('task.actions.dateLabel')}
		</div>
		<div class={isMobile.current ? 'grid grid-cols-4 gap-1' : 'flex items-center gap-1'}>
			<button
				type="button"
				class:mobile-control={isMobile.current}
				title={$t('common.today')}
				aria-label={$t('common.today')}
				aria-pressed={isToday}
				onclick={() => setDate(todayKey)}
				class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
				class:bg-accent={isToday}
			>
				<CalendarBlankIcon class="size-4 text-emerald-500" />
				{#if isMobile.current}<span>{$t('common.today')}</span>{/if}
			</button>
			<button
				type="button"
				class:mobile-control={isMobile.current}
				title={$t('common.tomorrow')}
				aria-label={$t('common.tomorrow')}
				aria-pressed={isTomorrow}
				onclick={() => setDate(shiftDayKey(todayKey, 1))}
				class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
				class:bg-accent={isTomorrow}
			>
				<SunIcon class="size-4 text-amber-500" />
				{#if isMobile.current}<span>{$t('common.tomorrow')}</span>{/if}
			</button>
			<button
				type="button"
				class:mobile-control={isMobile.current}
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
				{#if isMobile.current}<span
						>{inBacklog ? $t('task.actions.removeFromBacklog') : $t('task.actions.toBacklog')}</span
					>{/if}
			</button>
			<button
				type="button"
				class:mobile-control={isMobile.current}
				title={$t('task.actions.clearDate')}
				aria-label={$t('task.actions.clearDate')}
				disabled={task.dueAt === null}
				onclick={() => setDate(null)}
				class="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
			>
				<XIcon class="size-4" />
				{#if isMobile.current}<span>{$t('task.actions.clearLabel')}</span>{/if}
			</button>
		</div>
	</div>

	<div class="px-2 py-1.5">
		<div class="mb-1.5 text-xs font-medium text-muted-foreground">
			{$t('task.actions.dayPartLabel')}
		</div>
		<div class={isMobile.current ? 'grid grid-cols-4 gap-1' : 'flex items-center gap-1'}>
			{#each DAY_PARTS as opt (opt.part)}
				{@const Icon = opt.icon}
				{@const active = task.dayPart === opt.part}
				{@const label = $t(opt.labelKey)}
				<button
					type="button"
					class:mobile-control={isMobile.current}
					title={label}
					aria-label={label}
					aria-pressed={active}
					onclick={() => setDayPart(opt.part)}
					class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
					class:bg-accent={active}
				>
					<Icon class="size-4" />
					{#if isMobile.current}<span>{label}</span>{/if}
				</button>
			{/each}
			<button
				type="button"
				class:mobile-control={isMobile.current}
				title={$t('task.actions.clearDayPart')}
				aria-label={$t('task.actions.clearDayPart')}
				disabled={task.dayPart === 'none'}
				onclick={() => setDayPart('none')}
				class="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
			>
				<XIcon class="size-4" />
				{#if isMobile.current}<span>{$t('task.actions.clearLabel')}</span>{/if}
			</button>
		</div>
	</div>

	<div class="px-2 py-1.5">
		<div class="mb-1.5 text-xs font-medium text-muted-foreground">
			{$t('task.actions.priorityLabel')}
		</div>
		<div class={isMobile.current ? 'grid grid-cols-4 gap-1' : 'flex items-center gap-1'}>
			{#each PRIORITY_ORDER as p (p)}
				{@const active = task.priority === p}
				<button
					type="button"
					class:mobile-control={isMobile.current}
					title={priorityLocked ? $t('task.actions.priorityLockedTooltip') : PRIORITY_LABEL[p]}
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
					{#if isMobile.current}<span>{PRIORITY_LABEL[p]}</span>{/if}
				</button>
			{/each}
		</div>
		{#if priorityLocked}
			<div class="mt-1 text-[10px] text-muted-foreground">
				{$t('task.actions.lockedByTroiki')}
			</div>
		{/if}
	</div>
{/snippet}

{#if isMobile.current}
	<Sheet.Root bind:open={menuOpen}>
		<Sheet.Trigger>
			{#snippet child({ props })}
				<Button
					{...props}
					size="sm"
					variant="ghost"
					class="size-8 p-0 text-muted-foreground hover:text-foreground"
					aria-label={$t('task.actions.ariaLabel')}
					onclick={(e: MouseEvent) => {
						e.stopPropagation();
						mobilePage = 'main';
						(props as { onclick?: (e: MouseEvent) => void }).onclick?.(e);
					}}
				>
					<DotsThreeIcon class="size-5" weight="bold" />
				</Button>
			{/snippet}
		</Sheet.Trigger>
		<Sheet.Content
			side="bottom"
			showCloseButton={false}
			class="max-h-[calc(100dvh-env(safe-area-inset-top)-0.5rem)] gap-0 overflow-hidden rounded-t-2xl text-sm"
		>
			<div class="flex shrink-0 items-start gap-2 border-b px-4 py-3">
				{#if mobilePage === 'more'}
					<Button
						variant="ghost"
						size="icon"
						aria-label={$t('common.back')}
						bind:ref={backButton}
						onclick={() => void showMobilePage('main')}
					>
						<ArrowLeftIcon class="size-5" />
					</Button>
				{/if}
				<div class="min-w-0 flex-1 py-1.5">
					<Sheet.Title class="text-base"
						>{mobilePage === 'more'
							? $t('task.actions.more')
							: $t('task.actions.ariaLabel')}</Sheet.Title
					>
					<Sheet.Description class="mt-1 line-clamp-2 break-words text-sm"
						>{task.title}</Sheet.Description
					>
				</div>
				<Sheet.Close>
					{#snippet child({ props })}
						<Button {...props} variant="ghost" size="icon" aria-label={$t('task.actions.close')}>
							<XIcon class="size-5" />
						</Button>
					{/snippet}
				</Sheet.Close>
			</div>
			<div
				bind:this={mobileContent}
				class="min-h-0 overflow-y-auto overscroll-contain px-2 pt-2 pb-[calc(0.5rem+env(safe-area-inset-bottom))]"
			>
				{#if mobilePage === 'main'}
					{#if !inInbox}
						{@render planningControls()}
						<div class="my-2 border-t"></div>
					{/if}
					{@render actionItems(mobileActions.filter((action) => !action.rare))}
					<button
						type="button"
						class="flex min-h-11 w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm hover:bg-accent focus-visible:outline-ring"
						bind:this={moreButton}
						onclick={() => void showMobilePage('more')}
					>
						<DotsThreeIcon class="size-4" />
						{$t('task.actions.more')}
						<CaretRightIcon class="ml-auto size-4" />
					</button>
					<div class="my-2 border-t"></div>
					<button
						type="button"
						class="flex min-h-11 w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm text-destructive hover:bg-destructive/10 focus-visible:outline-ring"
						onclick={() => {
							menuOpen = false;
							void deleteTask(task, mutator);
						}}
					>
						<TrashIcon class="size-4" />
						{$t('common.delete')}
					</button>
				{:else}
					{@render actionItems(mobileActions.filter((action) => action.rare))}
				{/if}
			</div>
		</Sheet.Content>
	</Sheet.Root>
{:else}
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
			{@render actionItems(primaryActions)}
			<DropdownMenu.Sub>
				<DropdownMenu.SubTrigger>
					<DotsThreeIcon class="size-4" />
					{$t('task.actions.moreSubmenu')}
				</DropdownMenu.SubTrigger>
				<DropdownMenu.SubContent class="min-w-[14rem]">
					{@render actionItems(extraActions)}
				</DropdownMenu.SubContent>
			</DropdownMenu.Sub>
			{#if !inInbox}
				<DropdownMenu.Separator />
				{@render planningControls()}
			{/if}
			<DropdownMenu.Separator />
			<DropdownMenu.Item variant="destructive" onclick={() => void deleteTask(task, mutator)}>
				<TrashIcon class="size-4" />
				{$t('common.delete')}
			</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Root>
{/if}

<MoveTaskDialog bind:open={moveOpen} {task} {mutator} {belongs} />
<MoveSectionDialog
	bind:open={moveSectionOpen}
	{task}
	{mutator}
	{belongs}
	sections={projectSectionsCtx?.sections}
/>
<DecomposeTaskDialog bind:open={decomposeOpen} {task} {mutator} />
<TemplateEditorDialog
	bind:open={templateEditorOpen}
	prefill={templateDraft}
	onSave={saveTemplate}
/>

<style>
	.mobile-control {
		width: 100%;
		height: auto;
		min-height: 3rem;
		flex-direction: column;
		gap: 0.25rem;
		padding: 0.375rem 0.125rem;
		font-size: 0.75rem;
		line-height: 1.25;
	}
</style>
