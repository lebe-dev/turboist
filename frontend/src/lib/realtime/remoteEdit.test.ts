import { describe, expect, it, vi } from 'vitest';
import { createRemoteEditWatcher } from './remoteEdit';

// createRemoteEditWatcher encapsulates the F3.4 open-card "updated remotely"
// affordance (US-3.1 AC3). A federation-origin change reaches the open editor on
// the same coarse SSE scope a normal refresh uses (the federation publish carries
// no origin, so it is NOT echo-suppressed — see service/federation/inbox_notify.go).
// The watcher decides, per incoming invalidation, whether to:
//   - run the normal refresh (entity not being edited — US-3.1 AC2), or
//   - hold the change behind a non-destructive notice (entity has in-flight edits
//     — US-3.1 AC3), exposing Reload / Keep-editing actions.

describe('createRemoteEditWatcher', () => {
	it('refreshes immediately when the editor is not dirty (US-3.1 AC2)', () => {
		const refresh = vi.fn();
		const w = createRemoteEditWatcher({ isDirty: () => false, refresh });

		w.onRemoteChange();

		// Not editing → behave like the normal F3.2 refresh, no notice.
		expect(refresh).toHaveBeenCalledTimes(1);
		expect(w.pending).toBe(false);
	});

	it('does NOT auto-overwrite a dirty editor — surfaces a notice instead (US-3.1 AC3)', () => {
		const refresh = vi.fn();
		const w = createRemoteEditWatcher({ isDirty: () => true, refresh });

		w.onRemoteChange();

		// In-flight edit must be preserved: no blind refresh.
		expect(refresh).not.toHaveBeenCalled();
		expect(w.pending).toBe(true);
	});

	it('reload() applies the latest and clears the notice', async () => {
		const refresh = vi.fn();
		const w = createRemoteEditWatcher({ isDirty: () => true, refresh });
		w.onRemoteChange();
		expect(w.pending).toBe(true);

		await w.reload();

		expect(refresh).toHaveBeenCalledTimes(1);
		expect(w.pending).toBe(false);
	});

	it('reload() prefers the explicit reload path over the background refresh', async () => {
		// The clean-editor background path (refresh) swallows errors; the explicit
		// Reload (reload) must run the surfacing path instead — they are distinct.
		const refresh = vi.fn();
		const reload = vi.fn(async () => {});
		const w = createRemoteEditWatcher({ isDirty: () => true, refresh, reload });
		w.onRemoteChange();

		await w.reload();

		expect(reload).toHaveBeenCalledTimes(1);
		expect(refresh).not.toHaveBeenCalled();
		expect(w.pending).toBe(false);
	});

	it('reload() KEEPS the notice up when the reload rejects (federation rejection path)', async () => {
		// Peer-deleted (410), read-only (403) or expired-session (401) reload: the
		// surfacing path has already toasted/mapped the failure. The banner — the
		// user's only signal — MUST persist so a failed reload is never silent.
		const refresh = vi.fn();
		const reload = vi.fn(async () => {
			throw new Error('gone');
		});
		const w = createRemoteEditWatcher({ isDirty: () => true, refresh, reload });
		w.onRemoteChange();
		expect(w.pending).toBe(true);

		await w.reload();

		expect(reload).toHaveBeenCalledTimes(1);
		// Reload failed → notice stays up; the user has NOT silently lost their signal.
		expect(w.pending).toBe(true);
	});

	it('reload() awaits the refresh and clears only after it resolves', async () => {
		// reload() must not clear the notice optimistically before the pull lands —
		// a reload that rejects after a tick must still leave the banner up.
		let reject!: (e: unknown) => void;
		const reload = vi.fn(
			() =>
				new Promise<void>((_, rej) => {
					reject = rej;
				})
		);
		const w = createRemoteEditWatcher({ isDirty: () => true, refresh: vi.fn(), reload });
		w.onRemoteChange();

		const done = w.reload();
		// Still pending while the pull is in flight.
		expect(w.pending).toBe(true);
		reject(new Error('403'));
		await done;
		expect(w.pending).toBe(true);
	});

	it('keepEditing() clears the notice WITHOUT refreshing — in-flight edit survives', () => {
		const refresh = vi.fn();
		const w = createRemoteEditWatcher({ isDirty: () => true, refresh });
		w.onRemoteChange();

		w.keepEditing();

		expect(refresh).not.toHaveBeenCalled();
		expect(w.pending).toBe(false);
	});

	it('coalesces multiple remote changes into a single notice', () => {
		const refresh = vi.fn();
		const w = createRemoteEditWatcher({ isDirty: () => true, refresh });
		w.onRemoteChange();
		w.onRemoteChange();
		w.onRemoteChange();
		expect(w.pending).toBe(true);
		expect(refresh).not.toHaveBeenCalled();
	});

	it('clear() resets the notice (e.g. on save/close) so it does not linger', () => {
		const refresh = vi.fn();
		const w = createRemoteEditWatcher({ isDirty: () => true, refresh });
		w.onRemoteChange();
		expect(w.pending).toBe(true);

		w.clear();

		expect(w.pending).toBe(false);
		expect(refresh).not.toHaveBeenCalled();
	});

	it('a change arriving while clean, then dirtying, keeps the editor — later remote change holds', () => {
		let dirty = false;
		const refresh = vi.fn();
		const w = createRemoteEditWatcher({ isDirty: () => dirty, refresh });

		// First remote change while clean → refresh.
		w.onRemoteChange();
		expect(refresh).toHaveBeenCalledTimes(1);
		expect(w.pending).toBe(false);

		// User starts editing; a second remote change must NOT clobber.
		dirty = true;
		w.onRemoteChange();
		expect(refresh).toHaveBeenCalledTimes(1);
		expect(w.pending).toBe(true);
	});
});
