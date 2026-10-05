<script lang="ts">
	import { resolve } from '$app/paths';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import PushPinIcon from 'phosphor-svelte/lib/PushPin';
	import PushPinSlashIcon from 'phosphor-svelte/lib/PushPinSlash';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import type { Project } from '$lib/api/types';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import TroikiTriggerIcon from './TroikiTriggerIcon.svelte';
	import { t } from '$lib/i18n';

	let {
		project,
		href,
		active,
		onTogglePin
	}: {
		project: Project;
		href: string;
		active: boolean;
		onTogglePin: (project: Project) => void;
	} = $props();

	const pinLabel = $derived(
		project.isPinned
			? $t('sidebar.unpinAria', { values: { name: project.title } })
			: $t('sidebar.pinAria', { values: { name: project.title } })
	);

	// The button lives inside the row, not inside the link, but a click still
	// bubbles up to the anchor on touch devices — stop it before it navigates.
	function togglePin(event: MouseEvent): void {
		event.preventDefault();
		event.stopPropagation();
		onTogglePin(project);
	}
</script>

<div
	class="group/project relative flex items-center rounded-md transition-colors hover:bg-sidebar-accent"
	class:bg-sidebar-accent={active}
>
	<a
		href={href as ReturnType<typeof resolve>}
		class="flex min-w-0 flex-1 items-center gap-2.5 px-2.5 py-2.5 text-[15px] text-muted-foreground transition-colors hover:text-foreground md:py-1 md:text-[13px]"
		class:text-foreground={active}
	>
		<FolderIcon
			class="size-4 shrink-0 opacity-90 md:size-3.5"
			style={`color: ${project.color}`}
			weight="fill"
		/>
		<span class="min-w-0 break-words">
			{project.title}{#if settingsStore.troikiEnabled && project.troikiCategory}<TroikiTriggerIcon
					class="ml-1.5 inline-block size-3 align-middle text-muted-foreground/50 md:size-2.5"
				/>{/if}{#if project.isPrivate && !settingsStore.publicView}<span
					class="inline-flex align-middle"
					title={$t('common.privateTooltip')}
					aria-label={$t('common.privateMarker')}
				><LockSimpleIcon class="ml-1.5 inline-block size-2.5 text-muted-foreground/40" /></span>{/if}
		</span>
	</a>
	<button
		type="button"
		class="mr-1 flex size-5 shrink-0 self-center items-center justify-center rounded text-muted-foreground opacity-0 transition-all hover:bg-sidebar-border hover:text-foreground focus:opacity-100 group-hover/project:opacity-100"
		onclick={togglePin}
		aria-label={pinLabel}
		title={pinLabel}
	>
		{#if project.isPinned}
			<PushPinSlashIcon class="size-3.5" />
		{:else}
			<PushPinIcon class="size-3.5" />
		{/if}
	</button>
</div>
