<script lang="ts">
	import { toast } from 'svelte-sonner';
	import {
		settingsStore,
		MIN_MAX_PINNED,
		MAX_MAX_PINNED
	} from '$lib/stores/settings.svelte';
	import { t } from '$lib/i18n';

	let tasksDraft = $state(settingsStore.maxPinnedTasks);
	let projectsDraft = $state(settingsStore.maxPinnedProjects);
	let busy = $state(false);

	// A cleared or half-typed number input yields NaN — treat it as "not a valid
	// value yet" so Save stays disabled instead of PATCHing garbage.
	function isValid(v: number): boolean {
		return Number.isInteger(v) && v >= MIN_MAX_PINNED && v <= MAX_MAX_PINNED;
	}

	const dirty = $derived(
		tasksDraft !== settingsStore.maxPinnedTasks ||
			projectsDraft !== settingsStore.maxPinnedProjects
	);
	const valid = $derived(isValid(tasksDraft) && isValid(projectsDraft));

	// The settings store may still be loading when this section mounts, and its
	// values change again on every successful save. Re-seed the drafts whenever
	// the server-known values move, so the form never shows stale defaults.
	// Guarded per field: an edit to one input must not stop the other from
	// picking up a new server value, and a store update must never wipe a number
	// the user is still editing.
	const lastSeen = {
		tasks: settingsStore.maxPinnedTasks,
		projects: settingsStore.maxPinnedProjects
	};
	$effect(() => {
		const tasks = settingsStore.maxPinnedTasks;
		if (tasks !== lastSeen.tasks) {
			if (tasksDraft === lastSeen.tasks) tasksDraft = tasks;
			lastSeen.tasks = tasks;
		}
		const projects = settingsStore.maxPinnedProjects;
		if (projects !== lastSeen.projects) {
			if (projectsDraft === lastSeen.projects) projectsDraft = projects;
			lastSeen.projects = projects;
		}
	});

	async function save(): Promise<void> {
		if (!valid) {
			toast.error(
				$t('settings.menu.pinned.toastRange', {
					values: { min: MIN_MAX_PINNED, max: MAX_MAX_PINNED }
				})
			);
			return;
		}
		busy = true;
		try {
			if (tasksDraft !== settingsStore.maxPinnedTasks) {
				await settingsStore.setMaxPinnedTasks(tasksDraft);
			}
			if (projectsDraft !== settingsStore.maxPinnedProjects) {
				await settingsStore.setMaxPinnedProjects(projectsDraft);
			}
			toast.success($t('settings.menu.pinned.toastSaved'));
		} catch (err) {
			// Pull the drafts back to what the server actually stores — with two
			// PATCHes the first may have landed and the second not.
			tasksDraft = settingsStore.maxPinnedTasks;
			projectsDraft = settingsStore.maxPinnedProjects;
			const message = err instanceof Error ? err.message : $t('settings.menu.pinned.toastFailed');
			toast.error(message);
		} finally {
			busy = false;
		}
	}
</script>

<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex flex-col gap-0.5">
		<h2 class="text-sm font-semibold">{$t('settings.menu.pinned.heading')}</h2>
		<p class="text-xs text-muted-foreground">{$t('settings.menu.pinned.description')}</p>
	</div>

	<div class="grid gap-3 sm:grid-cols-2">
		<label class="flex flex-col gap-1.5">
			<span class="text-xs font-medium text-muted-foreground">
				{$t('settings.menu.pinned.tasksLabel')}
			</span>
			<input
				type="number"
				inputmode="numeric"
				min={MIN_MAX_PINNED}
				max={MAX_MAX_PINNED}
				step="1"
				bind:value={tasksDraft}
				class="rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
			/>
		</label>
		<label class="flex flex-col gap-1.5">
			<span class="text-xs font-medium text-muted-foreground">
				{$t('settings.menu.pinned.projectsLabel')}
			</span>
			<input
				type="number"
				inputmode="numeric"
				min={MIN_MAX_PINNED}
				max={MAX_MAX_PINNED}
				step="1"
				bind:value={projectsDraft}
				class="rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
			/>
		</label>
	</div>

	<p class="text-xs text-muted-foreground">
		{$t('settings.menu.pinned.hint', {
			values: { min: MIN_MAX_PINNED, max: MAX_MAX_PINNED }
		})}
	</p>

	<div class="flex justify-end pt-1">
		<button
			type="button"
			onclick={save}
			disabled={!dirty || !valid || busy}
			class="inline-flex items-center gap-1 rounded-md bg-foreground px-3 py-1.5 text-xs font-medium text-background shadow-sm transition-colors hover:bg-foreground/90 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50"
		>
			{$t('settings.menu.pinned.save')}
		</button>
	</div>
</section>
