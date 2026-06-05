export const meta = {
  name: 'federation-impl',
  description: 'Implement one phase of FEDERATION-IMPLEMENTATION-PLAN.md milestone-by-milestone (tests-first impl -> lint/test gate with bounded fix loop -> parallel review -> post-review retest -> integration gate + report). Pass the phase number via args (default 0).',
  whenToUse: 'Drive the turboist Federation v1 implementation one phase at a time. args = phase number 0..7 (or {phase:n}). Milestones inside a phase are implemented sequentially because they share files (errors.go, server.go, config.go) and require strictly increasing goose migration numbers.',
  phases: [
    { title: 'Implement', detail: 'tests-first implementation of each milestone, in dependency order' },
    { title: 'Test', detail: 'just lint + focused tests, bounded fix loop' },
    { title: 'Review', detail: 'code-reviewer + silent-failure-hunter per milestone, fix blocking findings' },
    { title: 'Synthesis', detail: 'just test-all integration gate + written phase report' },
  ],
}

// ---- Phase -> ordered milestones (valid topological order; deps are intra-phase only) ----
const PHASE_PLAN = {
  0: [
    { id: 'F0.1', title: 'Offline-sync overlay: client_id + deleted_at on the 5 synced entities (migration 024)', deps: [] },
    { id: 'F0.2', title: 'Comments + checklist_items SCHEMA (deferrable, schema-only) (migrations 025, 026)', deps: ['F0.1'] },
    { id: 'F0.3', title: 'Instance trust plane: Ed25519 keypair, display_name, .well-known, HTTP-signature middleware (Must-grade checks), nonce cache, peer-key cache, canonical JSON (migration 027)', deps: [] },
    { id: 'F0.4', title: 'Protocol version negotiation CORE (pulled forward)', deps: ['F0.3'] },
  ],
  1: [
    { id: 'F1.1', title: 'Federation core schema + per-project enable (migrations 028, 029)', deps: [] },
    { id: 'F1.2', title: 'Invite creation: ULID id, fragment-secret link, hashed storage, expiry/max_uses', deps: ['F1.1'] },
    { id: 'F1.3', title: 'Invite list / status derivation / revoke / delete', deps: ['F1.2'] },
    { id: 'F1.4', title: 'Peers list with status / last-contact / stale & pending metrics', deps: ['F1.1'] },
  ],
  2: [
    { id: 'F2.1', title: 'Join landing page + invite parsing + cross-instance redirect', deps: [] },
    { id: 'F2.2', title: 'Handshake (owner + joiner): invite validation, signed handshake body, key + display_name exchange', deps: ['F2.1'] },
    { id: 'F2.3', title: 'Snapshot bootstrap: buffer-first NDJSON owner build + joiner apply with TTL token (migration 030)', deps: ['F2.2'] },
    { id: 'F2.4', title: 'Federated project surfaces in joiner UI: origin + role badges, read-only enforcement', deps: ['F2.3'] },
  ],
  3: [
    { id: 'F3.1', title: 'FederationService: transactional outbox emit + per-field LWW inbox apply', deps: [] },
    { id: 'F3.2a', title: 'Per-event payload validation: per-event signature, author/origin equality, HLC clock-skew (Must security)', deps: ['F3.1'] },
    { id: 'F3.2', title: 'Outbox publisher + inbox endpoint + pull/recovery + push <5s', deps: ['F3.1', 'F3.2a'] },
    { id: 'F3.3', title: 'Soft-delete tombstones, cascade child tombstones, resurrection prevention, retention GC, stale-pull 410 emit', deps: ['F3.2'] },
    { id: 'F3.4', title: 'Open-card "updated remotely" notice (non-destructive remote-edit affordance)', deps: ['F3.2'] },
  ],
  4: [
    { id: 'F4.1', title: 'Pull/recovery loop: auto catch-up after short offline', deps: [] },
    { id: 'F4.2', title: '410 Gone CONSUME -> snapshot re-bootstrap preserving unsent outbox', deps: ['F4.1'] },
    { id: 'F4.3', title: 'Federation sync-status read API + UI indicator', deps: ['F4.1'] },
    { id: 'F4.4', title: 'Outbox backpressure: retry classification + batch chunking + inbound rate limit / 413', deps: ['F4.1'] },
  ],
  5: [
    { id: 'F5.1', title: 'Peer permissions + outgoing fan-out filtering + hub-and-spoke re-broadcast (migration 032)', deps: [] },
    { id: 'F5.2', title: 'Incoming write-enforcement for read-only peers + UI edit lockout', deps: ['F5.1'] },
    { id: 'F5.3', title: 'Pause / resume peer (non-destructive)', deps: ['F5.1'] },
    { id: 'F5.4', title: 'Permanent revoke peer (+ federation_revoke event, irreversible) (migration 033)', deps: ['F5.1', 'F5.2', 'F5.3'] },
    { id: 'F5.5', title: 'Voluntary leave (federation_leave event)', deps: ['F5.4'] },
    { id: 'F5.6a', title: 'Owner-dead read-only fallback (Must-grade half of US-6.5)', deps: ['F5.1', 'F5.2'] },
    { id: 'F5.6b', title: 'Key-change detection + manual trust (migration 034)', deps: ['F5.1', 'F5.2', 'F5.4'] },
  ],
  6: [
    { id: 'F6.1', title: 'Protocol forward-compatibility: unknown-field decoding + relay integrity (migration 031)', deps: [] },
    { id: 'F6.2', title: 'Security hardening tie-off + threat-model documentation', deps: [] },
    { id: 'F6.3', title: 'Federation audit log + signature-failure attack alert (migration 035)', deps: ['F6.2'] },
    { id: 'F6.4', title: 'Federation visibility: overview API + "visible to N peers" badges + new-task hint with explicit instance list', deps: [] },
    { id: 'F6.5', title: 'Ops: health, Prometheus metrics, rate-limit, retention config, federation-aware backup (migration 036)', deps: [] },
  ],
  7: [
    { id: 'F7.1', title: 'Two/three-instance in-process integration harness', deps: [] },
    { id: 'F7.2', title: 'HLC correctness table-tests', deps: [] },
    { id: 'F7.3', title: '3-way concurrent-edit convergence', deps: ['F7.1'] },
    { id: 'F7.4', title: '60-day-offline pull->snapshot fallback + clock-skew', deps: ['F7.1'] },
    { id: 'F7.5', title: 'NFR-2 crash-safety', deps: [] },
    { id: 'F7.6', title: 'NFR-1 performance benchmarks', deps: [] },
    { id: 'F7.7', title: 'NFR-3/NFR-4 security + single-binary deploy (incl. client_golang CGO-free verification)', deps: [] },
  ],
}

const REPO = '/Users/eugene/pro/turboist'
const PLAN = REPO + '/FEDERATION-IMPLEMENTATION-PLAN.md'

const IMPL_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['milestone', 'status', 'summary', 'filesChanged'],
  properties: {
    milestone: { type: 'string' },
    status: { type: 'string', enum: ['complete', 'partial', 'blocked'] },
    summary: { type: 'string', description: 'What was implemented, 3-6 sentences.' },
    filesChanged: { type: 'array', items: { type: 'string' }, description: 'Repo-relative paths created or edited.' },
    testsAdded: { type: 'array', items: { type: 'string' }, description: 'Test names / files added, mapped to the milestone AC ids.' },
    focusedTestCommands: { type: 'array', items: { type: 'string' }, description: 'The exact `just` commands that exercise this milestone (e.g. just test ./internal/crypto, just test-frontend join).' },
    deviations: { type: 'array', items: { type: 'string' }, description: 'Any place you deviated from the plan, with reason.' },
    blockers: { type: 'array', items: { type: 'string' }, description: 'Anything you could not complete and why.' },
  },
}

const TEST_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['passed', 'commands', 'summary'],
  properties: {
    passed: { type: 'boolean' },
    commands: { type: 'array', items: { type: 'string' } },
    failingTests: { type: 'array', items: { type: 'string' } },
    output: { type: 'string', description: 'Last ~80 lines of failing output only; empty if passed.' },
    summary: { type: 'string' },
  },
}

const REVIEW_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['milestone', 'verdict', 'findings'],
  properties: {
    milestone: { type: 'string' },
    reviewer: { type: 'string' },
    verdict: { type: 'string', enum: ['pass', 'changes-needed'] },
    findings: {
      type: 'array',
      items: {
        type: 'object',
        additionalProperties: false,
        required: ['severity', 'issue'],
        properties: {
          severity: { type: 'string', enum: ['critical', 'high', 'medium', 'low'] },
          file: { type: 'string' },
          line: { type: 'string' },
          issue: { type: 'string' },
          suggestion: { type: 'string' },
        },
      },
    },
    acCoverage: {
      type: 'array',
      items: {
        type: 'object',
        additionalProperties: false,
        required: ['ac', 'covered'],
        properties: {
          ac: { type: 'string' },
          covered: { type: 'boolean' },
          note: { type: 'string' },
        },
      },
    },
  },
}

const COMMON_RULES = [
  'BINDING rules:',
  '- All commands go through `just` (just test <name>, just test-frontend <name>, just lint). Use ast-grep for structural search, not grep/rg.',
  '- Tests-first: write the tests named in the milestone Tests->AC mapping BEFORE the implementation. Confirm they fail, then make them pass.',
  '- Follow the plan §3 (Key Reconciliation Decisions) DEVIATE rows exactly: keep int64 AUTOINCREMENT PKs and ADD client_id TEXT UNIQUE (ULID); every NEW federation SQL table is federation_*/federated_* prefixed and MUST NOT reuse the names inbox/outbox (the GTD `inbox` table already exists at 001_schema.sql:6); goose migration numbers are the next free sequential number (run `ls internal/db/migrations/` first — 023 is the real max on disk); wire timestamps are TEXT ISO-8601 UTC via model.FormatUTC; HLC physical_ms comes from the SAME time.Now() that writes updated_at; node_id is a stable generated install UUID, not BASE_URL host; the signed transport canonical string is METHOD\\nRequest().URI().Path()\\ninstance_url\\ntimestamp\\nnonce\\nSHA256(body) (concrete path, NOT the Fiber route template); constant-time compares via crypto/subtle.',
  '- Read /Users/eugene/pro/turboist/CLAUDE.md and the matching files in .claude/rules/ for EVERY path you touch (go-handlers.md for internal/httpapi/**, go-testing.md for *_test.go, frontend-api.md for lib/api/**, svelte-stores.md for lib/stores/**, svelte-components.md for components/** and routes/**).',
  '- Every user-visible string lands in BOTH frontend/locales/en.json and ru.json and is accessed via $t(). English is the source of truth.',
  '- Edit shared files (internal/httpapi/errors.go, server.go, internal/config/config.go, go.mod) ADDITIVELY so later milestones in this phase can extend them; do not remove unrelated code.',
  '- Match surrounding code style: named exports, early returns over nested ifs.',
  '- Do NOT run git commit or git push under any circumstances — the user owns all commits.',
].join('\n')

function implPrompt(m, phaseMilestones, prior) {
  return [
    `Implement milestone ${m.id} — "${m.title}" — of the turboist Federation v1 plan.`,
    ``,
    `Authoritative spec: ${PLAN}. Read the ${m.id} section IN FULL, plus §1 (Executive Summary), §2 (Codebase Grounding), §3 (Key Reconciliation Decisions — the DEVIATE rows are binding), §5 (migrations table + internal/federation package layout), and the §7 Risk Register rows referenced by ${m.id}. The plan is the source of truth; this prompt only orchestrates.`,
    ``,
    `This run is implementing the phase whose milestones, in order, are: ${phaseMilestones.map(x => x.id).join(' -> ')}. Implement ONLY ${m.id} now; the rest are handled before/after you in this same working tree.`,
    prior ? `\nAlready implemented in this working tree (build on it; do NOT redo or revert it):\n${prior}\n` : ``,
    COMMON_RULES,
    ``,
    `Workflow for this milestone:`,
    `1. Run \`ls internal/db/migrations/\` to confirm the next free goose number before allocating any migration.`,
    `2. Write the tests from the milestone Tests->AC mapping first.`,
    `3. Implement the full milestone — backend AND frontend exactly as the ${m.id} section specifies.`,
    `4. Run the focused tests for what you changed via \`just test <pkg-or-name>\` and/or \`just test-frontend <name>\`, plus \`just lint\`, and iterate until green.`,
    `Then return the structured summary. Be honest in status/blockers — a partial that is clearly reported is better than an overstated complete.`,
  ].join('\n')
}

function testPrompt(m, impl) {
  const files = (impl.filesChanged || []).join(', ')
  const cmds = (impl.focusedTestCommands || []).join(' ; ')
  return [
    `Run the test + lint gate for federation milestone ${m.id} in ${REPO}. Do NOT modify any code — only run and report.`,
    `Files changed by the implementer: ${files || '(unknown — derive from `git diff --name-only`)'}.`,
    `Run, via just: (1) \`just lint\`; (2) the focused tests covering the changed files. The implementer suggested: ${cmds || '(none given — pick focused tests by changed package/file: Go `just test ./internal/<pkg>`, frontend `just test-frontend <name>`)'}.`,
    `Report passed=true only if BOTH lint and the focused tests are green. On failure include the failing test names and only the last ~80 lines of relevant output.`,
  ].join('\n')
}

function fixPrompt(m, test) {
  return [
    `The test/lint gate for federation milestone ${m.id} is RED. Fix it.`,
    `Failing commands: ${(test.commands || []).join(' ; ')}.`,
    `Failing tests: ${(test.failingTests || []).join(', ') || '(see output)'}.`,
    `Output tail:\n${test.output || test.summary || ''}`,
    ``,
    `Fix the implementation (or the test, if the test itself is wrong) so \`just lint\` and the focused tests pass. Stay faithful to ${PLAN} and CLAUDE.md. Do NOT git commit. Re-run the focused tests + lint and confirm green before returning a short note on what you changed.`,
  ].join('\n')
}

function reviewPrompt(m, impl, lens) {
  const files = (impl.filesChanged || []).join(', ')
  return [
    `Review the implementation of federation milestone ${m.id} — "${m.title}".`,
    `Focus ONLY on these changed files: ${files || '(derive via `git diff --name-only`)'}. Run \`git diff -- <those files>\` to see the actual changes.`,
    `Spec: ${PLAN} (read the ${m.id} section, especially its Tests->AC mapping and any §3 DEVIATE rows it depends on). Also enforce ${REPO}/CLAUDE.md and the matching .claude/rules/ files.`,
    lens === 'silent-failure'
      ? `Your lens: silent failures / inadequate error handling / inappropriate fallbacks — federation is heavy on signature checks, soft-delete read filters, LWW skips, retry/backoff, and 401/403/410 paths. Flag any swallowed error, missing constant-time compare, fallback that masks a rejection, or read path that forgets WHERE deleted_at IS NULL.`
      : `Your lens: adherence to the plan + project conventions. Verify the federation_*/federated_* table-name prefix, correct sequential goose numbering, the pinned canonical signing string, TEXT ISO-8601 timestamps, i18n parity (every new string in both en.json and ru.json), and that EACH AC in the ${m.id} Tests->AC list has a corresponding test (report it in acCoverage).`,
    `Set verdict=changes-needed only if there is at least one critical/high finding. Set reviewer to your lens.`,
  ].join('\n')
}

function reviewFixPrompt(m, blocking) {
  const list = blocking.map(f => `- [${f.severity}] ${f.file || ''}${f.line ? ':' + f.line : ''} — ${f.issue}${f.suggestion ? ' (suggestion: ' + f.suggestion + ')' : ''}`).join('\n')
  return [
    `Apply fixes for the BLOCKING (critical/high) review findings on federation milestone ${m.id}:`,
    list,
    ``,
    `Fix each one faithfully to ${PLAN} and CLAUDE.md. Add/adjust tests if a finding is about missing AC coverage. Run \`just lint\` and the focused tests after, confirm green. Do NOT git commit. Return a short note of what you changed.`,
  ].join('\n')
}

function reportPrompt(phase, results, finalGate) {
  const blob = JSON.stringify(results.map(r => ({
    id: r.id,
    title: r.title,
    implStatus: r.impl.status,
    summary: r.impl.summary,
    filesChanged: r.impl.filesChanged,
    testsAdded: r.impl.testsAdded,
    deviations: r.impl.deviations,
    blockers: r.impl.blockers,
    testPassed: r.test.passed,
    failingTests: r.test.failingTests,
    reviews: r.reviews.map(x => ({ reviewer: x.reviewer, verdict: x.verdict, findings: x.findings, acCoverage: x.acCoverage })),
  })), null, 2)
  return [
    `Write a Phase ${phase} implementation report for the turboist federation work to ${REPO}/FEDERATION-PHASE-${phase}-REPORT.md (overwrite if present).`,
    `Structure: a per-milestone section (status, what landed, files, tests added, deviations, blockers, review verdicts + any unresolved critical/high findings, AC coverage gaps), then a "Phase gate" section with the just test-all + just lint result, then a "Manual follow-ups before the next phase" checklist.`,
    `Final integration gate result (just lint + just test-all): ${JSON.stringify({ passed: finalGate.passed, failingTests: finalGate.failingTests, summary: finalGate.summary })}.`,
    `Per-milestone data:\n${blob}`,
    `Be candid about anything partial/blocked or any AC without a test — this report tells the human what to verify before phase ${phase + 1}. After writing the file, return a 4-6 sentence summary of the phase outcome.`,
  ].join('\n')
}

// ---------------- run ----------------
// Resolve the target phase robustly: the runtime may deliver args as a number, a
// numeric string, or an object {phase}. NEVER silently default to 0 — a mis-passed
// arg must fail fast rather than re-run Phase 0 (which is what happened when a bare
// scalar arrived as a string and the old `typeof === 'number'` check missed it).
function resolvePhase(a) {
  if (typeof a === 'number' && Number.isInteger(a)) return a
  if (a && typeof a === 'object') {
    if (typeof a.phase === 'number' && Number.isInteger(a.phase)) return a.phase
    if (typeof a.phase === 'string' && a.phase.trim() !== '' && Number.isInteger(Number(a.phase))) return Number(a.phase)
    return null
  }
  if (typeof a === 'string') {
    const s = a.trim()
    if (s === '') return null
    // The runtime delivers args as a string here — a bare "1" or a JSON-encoded
    // object like {"phase":1}. Try numeric first, then JSON-decode and recurse.
    if (Number.isInteger(Number(s))) return Number(s)
    try {
      return resolvePhase(JSON.parse(s))
    } catch (_e) {
      return null
    }
  }
  return null
}

const PHASE = resolvePhase(args)
if (PHASE === null) {
  throw new Error(`federation-impl: could not resolve a phase from args=${JSON.stringify(args)} (type ${typeof args}). Pass a phase number 0..7 (or {phase:n}).`)
}
const milestones = PHASE_PLAN[PHASE]
if (!milestones) {
  throw new Error(`federation-impl: unknown phase ${PHASE} (from args=${JSON.stringify(args)}). Valid phases: ${Object.keys(PHASE_PLAN).join(', ')}`)
}

log(`Federation implementation — Phase ${PHASE} (raw args=${JSON.stringify(args)}, type ${typeof args}): ${milestones.length} milestone(s) (${milestones.map(m => m.id).join(', ')}). Sequential: shared files + ordered goose migrations forbid parallel code-writing.`)

const results = []
const priorSummaries = []

for (const m of milestones) {
  log(`▶ ${m.id} — ${m.title}`)

  const impl = await agent(implPrompt(m, milestones, priorSummaries.join('\n')), {
    label: `impl:${m.id}`, phase: 'Implement', schema: IMPL_SCHEMA,
  })

  // Test/lint gate with a bounded fix loop.
  let test = await agent(testPrompt(m, impl), { label: `test:${m.id}`, phase: 'Test', schema: TEST_SCHEMA })
  let attempt = 0
  while (!test.passed && attempt < 2) {
    attempt++
    log(`  ${m.id}: gate red — fix attempt ${attempt}/2`)
    await agent(fixPrompt(m, test), { label: `fix:${m.id}#${attempt}`, phase: 'Test' })
    test = await agent(testPrompt(m, impl), { label: `retest:${m.id}#${attempt}`, phase: 'Test', schema: TEST_SCHEMA })
  }

  // Parallel review fan-out: conventions/AC lens + silent-failure lens.
  const reviews = (await parallel([
    () => agent(reviewPrompt(m, impl, 'conventions'), { label: `review:${m.id}`, phase: 'Review', schema: REVIEW_SCHEMA, agentType: 'pr-review-toolkit:code-reviewer' }),
    () => agent(reviewPrompt(m, impl, 'silent-failure'), { label: `silent:${m.id}`, phase: 'Review', schema: REVIEW_SCHEMA, agentType: 'pr-review-toolkit:silent-failure-hunter' }),
  ])).filter(Boolean)

  const blocking = reviews.flatMap(r => r.findings || []).filter(f => f.severity === 'critical' || f.severity === 'high')
  if (blocking.length) {
    log(`  ${m.id}: ${blocking.length} blocking review finding(s) — applying fixes + retest`)
    await agent(reviewFixPrompt(m, blocking), { label: `review-fix:${m.id}`, phase: 'Review' })
    test = await agent(testPrompt(m, impl), { label: `post-review-test:${m.id}`, phase: 'Test', schema: TEST_SCHEMA })
  }

  results.push({ id: m.id, title: m.title, impl, test, reviews })
  priorSummaries.push(`${m.id} (${impl.status}): ${impl.summary} [files: ${(impl.filesChanged || []).join(', ')}]`)
}

// Integration gate + written report.
phase('Synthesis')
const finalGate = await agent(
  `In ${REPO} run \`just lint\` then \`just test-all\`. Do NOT modify code. Report passed=true only if both are fully green; otherwise list failing test names and the last ~80 lines of relevant output.`,
  { label: 'final-gate', phase: 'Synthesis', schema: TEST_SCHEMA },
)

const reportSummary = await agent(reportPrompt(PHASE, results, finalGate), { label: 'report', phase: 'Synthesis' })

return {
  phase: PHASE,
  reportPath: `FEDERATION-PHASE-${PHASE}-REPORT.md`,
  finalGate: { passed: finalGate.passed, failingTests: finalGate.failingTests || [], summary: finalGate.summary },
  milestones: results.map(r => ({
    id: r.id,
    implStatus: r.impl.status,
    testPassed: r.test.passed,
    blockers: r.impl.blockers || [],
    reviewVerdicts: r.reviews.map(x => `${x.reviewer || '?'}:${x.verdict}`),
    unresolvedBlocking: r.reviews.flatMap(x => x.findings || []).filter(f => f.severity === 'critical' || f.severity === 'high').length,
  })),
  summary: reportSummary,
}
