import { describe, expect, it } from 'vitest';
import {
	federationRole,
	isJoinedFederated,
	isOwnerOffline,
	isReadOnlyFederated,
	isReadOnlyFederationError,
	peerNamesLabel,
	visiblePeers
} from './projectSurface';
import { ApiError } from '$lib/api/errors';
import type { Project } from '$lib/api/types';

function project(over: Partial<Project> = {}): Project {
	return {
		id: 1,
		contextId: 1,
		title: 'p',
		description: '',
		color: '#fff',
		status: 'open',
		projectType: 'generic',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		isFederated: false,
		originInstance: null,
		federationPermissions: null,
		isOwner: false,
		reBootstrappedAt: null,
		federationLost: false,
		federationLostReason: null,
		ownerOffline: false,
		peerInstances: [],
		labels: [],
		troikiCategory: null,
		clientId: '',
		deletedAt: null,
		createdAt: '',
		updatedAt: '',
		...over
	};
}

describe('isReadOnlyFederated (Federation v1 F2.4, US-2.4 AC4 UI leg)', () => {
	it('is true for a joined read-only federated project', () => {
		expect(
			isReadOnlyFederated(project({ isFederated: true, isOwner: false, federationPermissions: 'read' }))
		).toBe(true);
	});

	it('is false for a non-federated project', () => {
		expect(isReadOnlyFederated(project({ isFederated: false }))).toBe(false);
	});

	it("is false for the owner's own federated project (controls stay enabled)", () => {
		expect(
			isReadOnlyFederated(project({ isFederated: true, isOwner: true, federationPermissions: 'admin' }))
		).toBe(false);
	});

	it('is false for a joined writable federated project', () => {
		expect(
			isReadOnlyFederated(project({ isFederated: true, isOwner: false, federationPermissions: 'write' }))
		).toBe(false);
	});

	it('is true for a lost (revoked) copy even with a write grant (Federation v1 F5.4, US-6.2 AC3)', () => {
		expect(
			isReadOnlyFederated(
				project({
					isFederated: true,
					isOwner: false,
					federationPermissions: 'write',
					federationLost: true,
					federationLostReason: 'revoked'
				})
			)
		).toBe(true);
	});

	it('is true for an owner-dead lost copy (US-6.5 read-only fallback)', () => {
		expect(
			isReadOnlyFederated(
				project({
					isFederated: true,
					isOwner: false,
					federationPermissions: 'write',
					federationLost: true,
					federationLostReason: 'owner-dead'
				})
			)
		).toBe(true);
	});

	it('is false for a voluntarily-left copy — it becomes editable local (US-6.3 AC3)', () => {
		expect(
			isReadOnlyFederated(
				project({
					isFederated: true,
					isOwner: false,
					federationPermissions: 'read',
					federationLost: true,
					federationLostReason: 'left'
				})
			)
		).toBe(false);
	});

	it('is true for an instance_url_changed copy — read-only history (Federation v1 F6.5, US-8.5 AC2)', () => {
		expect(
			isReadOnlyFederated(
				project({
					isFederated: true,
					isOwner: false,
					federationPermissions: 'write',
					federationLost: true,
					federationLostReason: 'instance_url_changed'
				})
			)
		).toBe(true);
	});

	it("is true for the OWNER's own project marked instance_url_changed — its mappings are history too (US-8.5 AC2)", () => {
		expect(
			isReadOnlyFederated(
				project({
					isFederated: true,
					isOwner: true,
					federationPermissions: 'admin',
					federationLost: true,
					federationLostReason: 'instance_url_changed'
				})
			)
		).toBe(true);
	});

	it('does NOT lock a writable copy whose owner is offline (US-6.5 AC2 — edits queued not blocked)', () => {
		expect(
			isReadOnlyFederated(
				project({
					isFederated: true,
					isOwner: false,
					federationPermissions: 'write',
					ownerOffline: true
				})
			)
		).toBe(false);
	});
});

describe('isOwnerOffline (Federation v1 F5.6a, US-6.5 AC1)', () => {
	it('is true for a joined copy whose owner is unreachable', () => {
		expect(
			isOwnerOffline(project({ isFederated: true, isOwner: false, ownerOffline: true }))
		).toBe(true);
	});

	it('is false when the owner is fresh', () => {
		expect(
			isOwnerOffline(project({ isFederated: true, isOwner: false, ownerOffline: false }))
		).toBe(false);
	});

	it("is false for the owner's own project and non-federated projects", () => {
		expect(isOwnerOffline(project({ isFederated: true, isOwner: true, ownerOffline: true }))).toBe(false);
		expect(isOwnerOffline(project({ isFederated: false, ownerOffline: true }))).toBe(false);
	});

	it('is false for an already-lost copy (it has its own terminal surface, not "owner offline")', () => {
		expect(
			isOwnerOffline(
				project({
					isFederated: true,
					isOwner: false,
					ownerOffline: true,
					federationLost: true,
					federationLostReason: 'revoked'
				})
			)
		).toBe(false);
	});
});

describe('isJoinedFederated (Federation v1 F2.4, US-2.4 AC2)', () => {
	it('is true for any joined copy regardless of grade', () => {
		expect(isJoinedFederated(project({ isFederated: true, isOwner: false }))).toBe(true);
	});
	it("is false for the owner's own project and non-federated projects", () => {
		expect(isJoinedFederated(project({ isFederated: true, isOwner: true }))).toBe(false);
		expect(isJoinedFederated(project({ isFederated: false }))).toBe(false);
	});
});

describe('federationRole (Federation v1 F2.4, US-2.4 AC1/AC2)', () => {
	it('maps every federation state to its role label', () => {
		expect(federationRole(project({ isFederated: false }))).toBe('none');
		expect(federationRole(project({ isFederated: true, isOwner: true, federationPermissions: 'admin' }))).toBe('owner');
		expect(federationRole(project({ isFederated: true, isOwner: false, federationPermissions: 'write' }))).toBe('write');
		expect(federationRole(project({ isFederated: true, isOwner: false, federationPermissions: 'read' }))).toBe('read');
	});
});

describe('isReadOnlyFederationError (Federation v1 F2.4, US-2.4 AC4 graceful 403)', () => {
	it('is true for a 403 federation_read_only ApiError', () => {
		expect(isReadOnlyFederationError(new ApiError('federation_read_only', 'read-only', 403))).toBe(true);
	});

	it('is false for other ApiErrors and non-ApiErrors', () => {
		expect(isReadOnlyFederationError(new ApiError('forbidden', 'no', 403))).toBe(false);
		expect(isReadOnlyFederationError(new ApiError('not_found', 'missing', 404))).toBe(false);
		expect(isReadOnlyFederationError(new Error('boom'))).toBe(false);
		expect(isReadOnlyFederationError(null)).toBe(false);
	});
});

describe('visiblePeers + peerNamesLabel (Federation v1 F6.4, US-7.1 AC3)', () => {
	it('visiblePeers returns the project peerInstances array', () => {
		const peers = [{ instanceUrl: 'https://alice.example', displayName: 'Alice' }];
		expect(visiblePeers(project({ peerInstances: peers }))).toEqual(peers);
	});

	it('visiblePeers is empty for a non-federated project', () => {
		expect(visiblePeers(project({}))).toEqual([]);
	});

	it('peerNamesLabel renders the NAMED instance list, not a count (US-7.1 AC3)', () => {
		const label = peerNamesLabel([
			{ instanceUrl: 'https://alice.example', displayName: 'Alice' },
			{ instanceUrl: 'https://bob.example', displayName: 'Bob' }
		]);
		expect(label).toBe('Alice, Bob');
	});

	it('peerNamesLabel falls back to the instanceUrl when displayName is empty', () => {
		const label = peerNamesLabel([{ instanceUrl: 'https://noname.example', displayName: '' }]);
		expect(label).toBe('https://noname.example');
	});
});
