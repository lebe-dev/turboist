import { harpoon as harpoonApi } from '../api/endpoints/harpoon';
import { getApiClient } from '../api/client';
import type { HarpoonKind, HarpoonSlot } from '../api/types';

// HarpoonTarget describes where the jump button on a given entity should go:
// the other member of the pair, plus the visual direction (slot 0 → slot 1 is
// "down", slot 1 → slot 0 is "up").
export interface HarpoonTarget {
	slot: HarpoonSlot;
	direction: 'up' | 'down';
}

function createHarpoonStore() {
	let slots = $state<HarpoonSlot[]>([]);

	function indexOf(kind: HarpoonKind, id: number): number {
		return slots.findIndex((s) => s.kind === kind && s.id === id);
	}

	return {
		get slots(): HarpoonSlot[] {
			return slots;
		},

		isHarpooned(kind: HarpoonKind, id: number): boolean {
			return indexOf(kind, id) !== -1;
		},

		// target returns the slot to jump to from the given entity, or null when
		// the entity is not harpooned or has no partner slot.
		target(kind: HarpoonKind, id: number): HarpoonTarget | null {
			const i = indexOf(kind, id);
			if (i === -1) return null;
			const other = slots[i === 0 ? 1 : 0];
			if (!other) return null;
			return { slot: other, direction: i === 0 ? 'down' : 'up' };
		},

		async load(): Promise<void> {
			const state = await harpoonApi.get(getApiClient());
			slots = state.slots;
		},

		async attach(kind: HarpoonKind, id: number): Promise<void> {
			const state = await harpoonApi.attach(getApiClient(), kind, id);
			slots = state.slots;
		},

		async detach(kind: HarpoonKind, id: number): Promise<void> {
			const state = await harpoonApi.detach(getApiClient(), kind, id);
			slots = state.slots;
		},

		clear(): void {
			slots = [];
		}
	};
}

export const harpoonStore = createHarpoonStore();
