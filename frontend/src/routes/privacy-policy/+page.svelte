<script lang="ts">
	import LegalDocument from '$lib/components/LegalDocument.svelte';
	import { t } from '$lib/i18n';
	import content from '../../../../privacy-policy.md?raw';
</script>

<svelte:head><title>{$t('legal.privacy.title')}</title></svelte:head>

<LegalDocument title={$t('legal.privacy.title')} {content} />
