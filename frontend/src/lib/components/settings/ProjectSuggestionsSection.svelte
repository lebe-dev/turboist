<script lang="ts">
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from 'svelte-sonner';
	import { appSettingsStore } from '$lib/stores/appSettings.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { isProjectVisible } from '$lib/utils/visibility';
	import { t } from '$lib/i18n';
	import type { ProjectSuggestionRule } from '$lib/api/types';
	import { MAX_PROJECT_SUGGESTIONS } from '$lib/utils/projectSuggestions';

	let draft = $state<ProjectSuggestionRule[]>(
		appSettingsStore.projectSuggestions.map((r) => ({ ...r, projectIds: [...r.projectIds] }))
	);
	let busy = $state(false);

	// Only open, visible projects can be picked; already-referenced ones still
	// render by name through projectTitleById so an archived pick stays readable.
	const selectableProjects = $derived(
		projectsStore.items
			.filter((p) => p.status !== 'completed' && isProjectVisible(p, settingsStore.publicView))
			.toSorted((a, b) => a.title.localeCompare(b.title, undefined, { sensitivity: 'base' }))
	);
	const projectTitleById = $derived(new Map(projectsStore.items.map((p) => [p.id, p.title])));

	const dirty = $derived.by(() => {
		const a = draft;
		const b = appSettingsStore.projectSuggestions;
		if (a.length !== b.length) return true;
		for (let i = 0; i < a.length; i++) {
			if (a[i].mask !== b[i].mask || a[i].ignoreCase !== b[i].ignoreCase) return true;
			if (a[i].projectIds.length !== b[i].projectIds.length) return true;
			for (let j = 0; j < a[i].projectIds.length; j++) {
				if (a[i].projectIds[j] !== b[i].projectIds[j]) return true;
			}
		}
		return false;
	});

	// Set when a new rule is appended; consumed by the mask input's attach to autofocus it
	let pendingRuleFocus = $state(false);

	function addRule(): void {
		draft = [...draft, { mask: '', projectIds: [], ignoreCase: true }];
		pendingRuleFocus = true;
	}

	function focusNewMask(el: HTMLInputElement): void {
		// Only the visible layout (mobile vs desktop) has a non-null offsetParent
		if (pendingRuleFocus && el.offsetParent) {
			pendingRuleFocus = false;
			el.focus();
		}
	}

	function removeRule(idx: number): void {
		draft = draft.filter((_, i) => i !== idx);
	}

	function toggleRuleProject(ruleIdx: number, projectId: number): void {
		const rule = draft[ruleIdx];
		if (!rule) return;
		const ids = rule.projectIds.includes(projectId)
			? rule.projectIds.filter((id) => id !== projectId)
			: [...rule.projectIds, projectId];
		draft = draft.map((r, i) => (i === ruleIdx ? { ...r, projectIds: ids } : r));
	}

	async function save(): Promise<void> {
		const cleaned = draft.map((r) => ({
			mask: r.mask.trim(),
			projectIds: r.projectIds,
			ignoreCase: r.ignoreCase
		}));
		if (cleaned.some((r) => r.mask === '' || r.projectIds.length === 0)) {
			toast.error($t('settings.projectSuggestions.toastEmptyFields'));
			return;
		}
		busy = true;
		try {
			await appSettingsStore.setProjectSuggestions(cleaned);
			toast.success($t('settings.projectSuggestions.toastSaved'));
		} catch (err) {
			const message =
				err instanceof Error ? err.message : $t('settings.projectSuggestions.toastFailed');
			toast.error(message);
		} finally {
			busy = false;
		}
	}
</script>

<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex flex-col gap-0.5">
		<h2 class="text-sm font-semibold">{$t('settings.projectSuggestions.heading')}</h2>
		<p class="text-xs text-muted-foreground">
			{$t('settings.projectSuggestions.description', {
				values: { max: MAX_PROJECT_SUGGESTIONS }
			})}
		</p>
	</div>

	{#if draft.length === 0}
		<p class="text-sm text-muted-foreground">{$t('settings.projectSuggestions.empty')}</p>
	{:else}
		<div class="flex flex-col gap-2">
			<div
				class="hidden grid-cols-[1fr_1fr_auto_auto] items-center gap-2 px-1 text-[11px] font-medium text-muted-foreground sm:grid"
			>
				<span>{$t('settings.projectSuggestions.mask')}</span>
				<span>{$t('settings.projectSuggestions.projects')}</span>
				<span>{$t('settings.projectSuggestions.ignoreCase')}</span>
				<span class="sr-only">{$t('settings.projectSuggestions.remove')}</span>
			</div>
			{#each draft as rule, idx (idx)}
				{@const selectedTitles = rule.projectIds
					.map((id) => projectTitleById.get(id))
					.filter((n): n is string => !!n)}
				{#snippet projectPicker()}
					<DropdownMenu.Root>
						<DropdownMenu.Trigger
							class="flex w-full items-center justify-between gap-1 rounded-md border border-input bg-background px-2 py-1.5 text-sm shadow-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
						>
							<span
								class="truncate text-left {selectedTitles.length === 0
									? 'text-muted-foreground'
									: ''}"
							>
								{selectedTitles.length === 0
									? $t('settings.projectSuggestions.projectsPlaceholder')
									: selectedTitles.join(', ')}
							</span>
							<CaretDownIcon class="size-3.5 shrink-0 text-muted-foreground" />
						</DropdownMenu.Trigger>
						<DropdownMenu.Content class="max-h-60 w-56 overflow-auto">
							{#if selectableProjects.length === 0}
								<div class="px-2 py-1.5 text-xs text-muted-foreground">
									{$t('settings.projectSuggestions.noProjectsAvailable')}
								</div>
							{:else}
								{#each selectableProjects as project (project.id)}
									<DropdownMenu.CheckboxItem
										checked={rule.projectIds.includes(project.id)}
										onCheckedChange={() => toggleRuleProject(idx, project.id)}
										closeOnSelect={false}
									>
										{project.title}
									</DropdownMenu.CheckboxItem>
								{/each}
							{/if}
						</DropdownMenu.Content>
					</DropdownMenu.Root>
				{/snippet}
				<!-- mobile card -->
				<div class="flex flex-col gap-2 rounded-md border border-border p-3 sm:hidden">
					<div class="flex items-center gap-2">
						<input
							type="text"
							bind:value={rule.mask}
							{@attach (el) => {
								if (idx === draft.length - 1) focusNewMask(el);
							}}
							placeholder={$t('settings.projectSuggestions.maskPlaceholder')}
							class="min-w-0 flex-1 rounded-md border border-input bg-background px-2 py-1.5 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
						/>
						<Switch
							checked={rule.ignoreCase}
							onCheckedChange={(v) => (rule.ignoreCase = v)}
							aria-label={$t('settings.projectSuggestions.ignoreCase')}
						/>
						<button
							type="button"
							onclick={() => removeRule(idx)}
							aria-label={$t('settings.projectSuggestions.remove')}
							class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-destructive focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
						>
							<TrashIcon class="size-4" />
						</button>
					</div>
					{@render projectPicker()}
				</div>
				<!-- desktop row -->
				<div class="hidden grid-cols-[1fr_1fr_auto_auto] items-center gap-2 sm:grid">
					<input
						type="text"
						bind:value={rule.mask}
						{@attach (el) => {
							if (idx === draft.length - 1) focusNewMask(el);
						}}
						placeholder={$t('settings.projectSuggestions.maskPlaceholder')}
						class="rounded-md border border-input bg-background px-2 py-1.5 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
					/>
					{@render projectPicker()}
					<Switch
						checked={rule.ignoreCase}
						onCheckedChange={(v) => (rule.ignoreCase = v)}
						aria-label={$t('settings.projectSuggestions.ignoreCase')}
					/>
					<button
						type="button"
						onclick={() => removeRule(idx)}
						aria-label={$t('settings.projectSuggestions.remove')}
						class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-destructive focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
					>
						<TrashIcon class="size-4" />
					</button>
				</div>
			{/each}
		</div>
	{/if}

	<div class="flex items-center justify-between gap-2 pt-1">
		<button
			type="button"
			onclick={addRule}
			class="inline-flex items-center gap-1 rounded-md border border-input bg-background px-3 py-1.5 text-xs font-medium shadow-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
		>
			<PlusIcon class="size-3.5" />
			{$t('settings.projectSuggestions.add')}
		</button>
		<button
			type="button"
			onclick={save}
			disabled={!dirty || busy}
			class="inline-flex items-center gap-1 rounded-md bg-foreground px-3 py-1.5 text-xs font-medium text-background shadow-sm transition-colors hover:bg-foreground/90 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50"
		>
			{$t('settings.projectSuggestions.save')}
		</button>
	</div>
</section>
