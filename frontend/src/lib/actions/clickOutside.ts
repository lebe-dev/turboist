export function clickOutside(node: HTMLElement, handler: () => void) {
	let current = handler;
	function onPointerDown(event: PointerEvent) {
		const target = event.target as Node | null;
		if (target && !node.contains(target)) current();
	}
	document.addEventListener('pointerdown', onPointerDown, true);
	return {
		update(next: () => void) {
			current = next;
		},
		destroy() {
			document.removeEventListener('pointerdown', onPointerDown, true);
		}
	};
}
