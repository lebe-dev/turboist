<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import { Button } from '$lib/components/ui/button';
	import PriorityPicker from '$lib/components/task/PriorityPicker.svelte';
	import DayPartPicker from '$lib/components/task/DayPartPicker.svelte';
	import LabelPicker from '$lib/components/label/LabelPicker.svelte';
	import type { DayPart, Priority, TaskTemplate, TaskTemplateInput } from '$lib/api/types';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { t } from '$lib/i18n';

	let {
		open = $bindable(false),
		template = null,
		prefill = null,
		onSave
	}: {
		open?: boolean;
		template?: TaskTemplate | null;
		// Initial values for a brand-new template (e.g. captured from a task).
		// Unlike `template`, it does not switch the dialog into edit mode.
		prefill?: TaskTemplate | null;
		onSave?: (input: TaskTemplateInput) => void | Promise<void>;
	} = $props();

	interface SubtaskDraft {
		key: number;
		title: string;
		description: string;
		priority: Priority;
		dayPart: DayPart;
		labelIds: number[];
	}

	let name = $state('');
	let description = $state('');
	let priority = $state<Priority>('no-priority');
	let dayPart = $state<DayPart>('none');
	let labelIds = $state<number[]>([]);
	let subtasks = $state<SubtaskDraft[]>([]);
	let submitting = $state(false);
	let keySeq = 0;

	function newSubtask(): SubtaskDraft {
		return { key: keySeq++, title: '', description: '', priority: 'no-priority', dayPart: 'none', labelIds: [] };
	}

	function loadFrom(tpl: TaskTemplate | null): void {
		name = tpl?.name ?? '';
		description = tpl?.description ?? '';
		priority = tpl?.priority ?? 'no-priority';
		dayPart = tpl?.dayPart ?? 'none';
		labelIds = tpl?.labels.map((l) => l.id) ?? [];
		subtasks =
			tpl?.subtasks.map((st) => ({
				key: keySeq++,
				title: st.title,
				description: st.description,
				priority: st.priority,
				dayPart: st.dayPart,
				labelIds: st.labels.map((l) => l.id)
			})) ?? [];
	}

	let prevOpen = false;
	$effect(() => {
		if (open && !prevOpen) loadFrom(template ?? prefill);
		prevOpen = open;
	});

	function addSubtask(): void {
		subtasks = [...subtasks, newSubtask()];
	}

	function removeSubtask(key: number): void {
		subtasks = subtasks.filter((s) => s.key !== key);
	}

	const canSave = $derived(
		name.trim().length > 0 && subtasks.every((s) => s.title.trim().length > 0)
	);

	async function submit(e: Event): Promise<void> {
		e.preventDefault();
		if (!canSave || submitting) return;
		submitting = true;
		try {
			const input: TaskTemplateInput = {
				name: name.trim(),
				description: description.trim(),
				priority,
				dayPart,
				labelIds,
				subtasks: subtasks.map((s) => ({
					title: s.title.trim(),
					description: s.description.trim(),
					priority: s.priority,
					dayPart: s.dayPart,
					labelIds: s.labelIds
				}))
			};
			await onSave?.(input);
			open = false;
		} finally {
			submitting = false;
		}
	}
</script>

<DialogPrimitive.Root bind:open>
	<DialogPrimitive.Portal>
		<DialogPrimitive.Overlay
			class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
		/>
		<DialogPrimitive.Content
			class="fixed left-1/2 top-[10%] z-50 flex max-h-[82vh] w-[calc(100%-2rem)] max-w-xl -translate-x-1/2 flex-col overflow-hidden rounded-xl border border-border bg-popover text-popover-foreground shadow-xl outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
		>
			<DialogPrimitive.Title class="shrink-0 border-b border-border px-5 py-3 text-sm font-semibold">
				{template ? $t('settings.templates.editTitle') : $t('settings.templates.newTitle')}
			</DialogPrimitive.Title>
			<DialogPrimitive.Description class="sr-only">
				{$t('settings.templates.dialogDescription')}
			</DialogPrimitive.Description>

			<form onsubmit={submit} class="flex min-h-0 flex-1 flex-col">
				<div class="min-h-0 flex-1 overflow-y-auto px-5 py-4">
					<!-- Root task -->
					<div class="flex flex-col gap-2">
						<input
							bind:value={name}
							placeholder={$t('settings.templates.namePlaceholder')}
							aria-label={$t('settings.templates.nameLabel')}
							class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm font-medium shadow-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
						/>
						<textarea
							bind:value={description}
							placeholder={$t('settings.templates.descriptionPlaceholder')}
							rows="2"
							class="w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
						></textarea>
						<div class="flex flex-wrap items-center gap-2">
							<PriorityPicker bind:value={priority} />
							<DayPartPicker bind:value={dayPart} />
							<LabelPicker bind:value={labelIds} />
						</div>
					</div>

					<!-- Subtasks -->
					<div class="mt-5 flex flex-col gap-2">
						<h3 class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
							{$t('settings.templates.subtasksHeading')}
						</h3>
						{#each subtasks as subtask (subtask.key)}
							<div class="flex flex-col gap-2 rounded-md border border-border p-3">
								<div class="flex items-center gap-2">
									<input
										bind:value={subtask.title}
										placeholder={$t('settings.templates.subtaskTitlePlaceholder')}
										aria-label={$t('settings.templates.subtaskTitlePlaceholder')}
										class="min-w-0 flex-1 rounded-md border border-input bg-background px-2 py-1.5 text-sm shadow-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
									/>
									<button
										type="button"
										onclick={() => removeSubtask(subtask.key)}
										aria-label={$t('settings.templates.removeSubtask')}
										class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-destructive focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
									>
										<TrashIcon class="size-4" />
									</button>
								</div>
								<textarea
									bind:value={subtask.description}
									placeholder={$t('settings.templates.descriptionPlaceholder')}
									rows="1"
									class="w-full resize-y rounded-md border border-input bg-background px-2 py-1.5 text-xs shadow-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
								></textarea>
								<div class="flex flex-wrap items-center gap-2">
									<PriorityPicker bind:value={subtask.priority} />
									<DayPartPicker bind:value={subtask.dayPart} />
									<LabelPicker bind:value={subtask.labelIds} />
								</div>
							</div>
						{/each}
						<button
							type="button"
							onclick={addSubtask}
							class="inline-flex w-fit items-center gap-1 rounded-md border border-input bg-background px-3 py-1.5 text-xs font-medium shadow-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
						>
							<PlusIcon class="size-3.5" />
							{$t('settings.templates.addSubtask')}
						</button>
					</div>
				</div>

				<div class="flex shrink-0 items-center justify-end gap-2 border-t border-border bg-muted/30 px-5 py-3">
					<DialogPrimitive.Close>
						{#snippet child({ props })}
							<Button {...props} variant="ghost" size="sm" type="button">{$t('common.cancel')}</Button>
						{/snippet}
					</DialogPrimitive.Close>
					<Button type="submit" size="sm" disabled={!canSave || submitting}>
						{$t('common.save')}
					</Button>
				</div>
			</form>
		</DialogPrimitive.Content>
	</DialogPrimitive.Portal>
</DialogPrimitive.Root>
