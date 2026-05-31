<script lang="ts">
	import { Dialog as SheetPrimitive } from "bits-ui";

	let { ref = $bindable(null), ...restProps }: SheetPrimitive.CloseProps = $props();
</script>

<!-- Default to type="button" so a Close used inside a <form> (e.g. a Cancel
button in an edit dialog) never submits it. Callers can override via restProps. -->
<SheetPrimitive.Close bind:ref data-slot="sheet-close" type="button" {...restProps} />
