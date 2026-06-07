<script lang="ts">
	import UsersThreeIcon from 'phosphor-svelte/lib/UsersThree';
	import TicketIcon from 'phosphor-svelte/lib/Ticket';
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

<div class="flex flex-col gap-5">
	<section class="flex flex-col gap-2">
		<h3 class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
			<UsersThreeIcon class="size-3.5" />
			{$t('federation.peers.title')}
		</h3>
		<PeersTable {projectId} />
	</section>

	<section class="flex flex-col gap-2">
		<h3 class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
			<TicketIcon class="size-3.5" />
			{$t('federation.invite.list.title')}
		</h3>
		{#key reloadKey}
			<InvitesTable {projectId} {sessionLinks} />
		{/key}
	</section>
</div>
