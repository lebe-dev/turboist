<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import ConfirmDestructiveDialog from '$lib/components/dialog/ConfirmDestructiveDialog.svelte';
	import TemplateEditorDialog from './TemplateEditorDialog.svelte';
	import { templatesStore } from '$lib/stores/templates.svelte';
	import { templates as templatesApi } from '$lib/api/endpoints/templates';
	import { getApiClient } from '$lib/api/client';
	import { describeError } from '$lib/utils/taskActions';
	import type { TaskTemplate, TaskTemplateInput } from '$lib/api/types';
	import { toast } from 'svelte-sonner';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import PencilSimpleIcon from 'phosphor-svelte/lib/PencilSimple';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import ListChecksIcon from 'phosphor-svelte/lib/ListChecks';
	import { t } from '$lib/i18n';

	let editorOpen = $state(false);
	let editing = $state<TaskTemplate | null>(null);
	let confirmOpen = $state(false);
	let deleting = $state<TaskTemplate | null>(null);

	function openCreate(): void {
		editing = null;
		editorOpen = true;
	}

	function openEdit(template: TaskTemplate): void {
		editing = template;
		editorOpen = true;
	}

	async function save(input: TaskTemplateInput): Promise<void> {
		const client = getApiClient();
		try {
			const saved = editing
				? await templatesApi.update(client, editing.id, input)
				: await templatesApi.create(client, input);
			templatesStore.upsert(saved);
			toast.success($t('settings.templates.toastSaved'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.templates.toastSaveFailed')));
			throw err;
		}
	}

	function requestDelete(template: TaskTemplate): void {
		deleting = template;
		confirmOpen = true;
	}

	async function confirmDelete(): Promise<void> {
		if (!deleting) return;
		const id = deleting.id;
		try {
			await templatesApi.remove(getApiClient(), id);
			templatesStore.remove(id);
			toast.success($t('settings.templates.toastDeleted'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.templates.toastDeleteFailed')));
		} finally {
			deleting = null;
		}
	}
</script>

<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex items-start justify-between gap-3">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('settings.templates.heading')}</h2>
			<p class="text-xs text-muted-foreground">{$t('settings.templates.description')}</p>
		</div>
		<Button size="sm" onclick={openCreate} class="shrink-0">
			<PlusIcon class="size-3.5" />
			{$t('settings.templates.new')}
		</Button>
	</div>

	{#if templatesStore.items.length === 0}
		<p class="text-sm text-muted-foreground">{$t('settings.templates.empty')}</p>
	{:else}
		<div class="flex flex-col gap-1">
			{#each templatesStore.items as template (template.id)}
				<div class="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2">
					<div class="flex min-w-0 flex-col">
						<span class="truncate text-sm font-medium">{template.name}</span>
						<span class="flex items-center gap-1 text-xs text-muted-foreground">
							<ListChecksIcon class="size-3.5" />
							{$t('settings.templates.subtaskCount', { values: { count: template.subtasks.length } })}
						</span>
					</div>
					<div class="flex shrink-0 items-center gap-1">
						<button
							type="button"
							onclick={() => openEdit(template)}
							aria-label={$t('settings.templates.edit')}
							title={$t('settings.templates.edit')}
							class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
						>
							<PencilSimpleIcon class="size-4" />
						</button>
						<button
							type="button"
							onclick={() => requestDelete(template)}
							aria-label={$t('settings.templates.delete')}
							title={$t('settings.templates.delete')}
							class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-destructive focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
						>
							<TrashIcon class="size-4" />
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</section>

<TemplateEditorDialog bind:open={editorOpen} template={editing} onSave={save} />
<ConfirmDestructiveDialog
	bind:open={confirmOpen}
	title={$t('settings.templates.confirmDeleteTitle')}
	description={deleting
		? $t('settings.templates.confirmDeleteNamed', { values: { name: deleting.name } })
		: $t('settings.templates.confirmDeleteTitle')}
	onConfirm={confirmDelete}
/>
