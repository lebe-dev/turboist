<script lang="ts">
	import LegalDocument from '$lib/components/LegalDocument.svelte';
	import { t } from '$lib/i18n';
	import content from '../../../../ToS.md?raw';
</script>

<svelte:head><title>{$t('legal.tos.title')}</title></svelte:head>

<LegalDocument title={$t('legal.tos.title')} {content} />
