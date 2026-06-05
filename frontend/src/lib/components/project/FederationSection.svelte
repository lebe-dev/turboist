<script lang="ts">
	import { t } from '$lib/i18n';
	import InvitesTable from './InvitesTable.svelte';
	import PeersTable from './PeersTable.svelte';

	let {
		projectId,
		// sessionLinks maps an invite id to the full share link captured at creation
		// time (the only moment the secret is in memory). InvitesTable offers
		// copy-link ONLY for these (US-1.3 AC4/AC5).
		sessionLinks = {},
		// reloadKey is bumped by the parent after a create so the invite list reloads.
		reloadKey = 0
	}: {
		projectId: number;
		sessionLinks?: Record<string, string>;
		reloadKey?: number;
	} = $props();
</script>

<div class="flex flex-col gap-4">
	<div>
		<h3 class="mb-2 text-sm font-medium text-muted-foreground">
			{$t('federation.peers.title')}
		</h3>
		<PeersTable {projectId} />
	</div>
	<div>
		<h3 class="mb-2 text-sm font-medium text-muted-foreground">
			{$t('federation.invite.list.title')}
		</h3>
		{#key reloadKey}
			<InvitesTable {projectId} {sessionLinks} />
		{/key}
	</div>
</div>
