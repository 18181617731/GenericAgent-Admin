# Inspection 02: action collaboration gap review

Date: 2026-08-09
Stage: inspection (no product implementation changes in this stage)
Perspective: end user delegating consequential work to an Agent, plus API/test reviewer
Baseline reviewed: `d8708fe feat: add persistent approval workboard`

## Verdict

The slice clears the minimum technical bar: it is writable, persistent, guarded by the existing dangerous-action contract, responsive, and not a restoration of the reverted read-only Agent Cockpit. It does **not yet clear the product bar in the objective**. In its present form a user can create and move a generic Kanban card, but cannot understand an Agent proposal, approve or return it with structured direction, inspect evidence, or accept an outcome. The next improvement should deepen one card into an explicit human/Agent decision loop rather than add dashboard surface area.

## Evidence gathered

- Read current backend, UI, focused tests, project memory, API safety SOP, and the reverted `AgentCockpit` history.
- The persisted item schema has only `id`, `title`, `owner`, `risk`, `status`, `created_at`, and `updated_at`.
- Static capability matrix: no contract fields, proposal model, evidence model, instruction model, event history, detail view, decision reason, or optimistic-concurrency token.
- `go test -race ./internal/api -run Workboard -count=10` passed.
- Focused web test command passed; existing Workboard coverage is 2 UI tests (create and one-stage move).
- The reverted Cockpit was session/activity aggregation with links back to chat. The new Workboard is materially different because it writes state, but its arrows still encode workflow mechanics rather than collaboration decisions.

## Findings, ranked

### P0 product gap: there is no task contract

A card title, owner, and risk cannot answer: what is the intended outcome, what constraints bind the Agent, and what proves completion? The user cannot safely delegate from this screen. Add a concise contract with at least outcome and acceptance criteria; constraints can be optional in the next vertical slice.

Acceptance standard:
- A user can create a task with an outcome and at least one acceptance criterion.
- The contract remains visible when making any approval decision.
- Empty/oversized values are rejected consistently by API and UI.

### P0 product gap: `review` is a column, not an approval interaction

The only controls are move-back and move-forward arrows. Moving `review -> done` is indistinguishable from approving; moving `review -> active` is indistinguishable from returning. Neither captures who decided, why, nor what the Agent must do next. This fails the requested human approval/return and structured instruction feedback loop.

Acceptance standard:
- In review, explicit `Approve` and `Return with instructions` actions replace generic arrows.
- Return requires a non-empty instruction and moves the item back to active.
- Approval records a decision event and advances only through a semantic API action.
- Confirmation copy previews the action and affected task, not a generic stage movement.

### P0 product gap: no Agent proposal or evidence can be reviewed

There is no structured proposal, claimed result, artifact/evidence reference, or acceptance checklist. Consequently review has no reviewable content and done has no defensible meaning.

Acceptance standard:
- An Agent-facing API action can submit a proposal/result summary and evidence entries.
- Evidence is rendered as data (label, kind, reference), not trusted HTML.
- Approval is rejected unless required acceptance criteria are addressed and at least one evidence item exists (or the contract explicitly permits none).

### P1 integrity gap: stale clients can overwrite a newer decision

The mutex prevents in-process data races, as confirmed by `-race`, but PATCH accepts only a target status. Two browser tabs can both act from the same stale card snapshot; the later request is evaluated against current persisted status without an expected revision. A stale backward action can undo a newer forward decision if it remains adjacent.

Acceptance standard:
- Each item exposes a monotonic `revision`.
- Every mutation supplies `expected_revision`; mismatch returns HTTP 409 with the current item.
- UI explains the conflict and reloads without silently discarding the user's typed return instruction.

### P1 audit gap: transitions destroy decision context

Only the latest status and timestamp survive. There is no event log for proposal submitted, approval, return instruction, evidence submission, or acceptance. For a consequential Agent action workbench, evidence and decision history are core state rather than decoration.

Acceptance standard:
- Append-only typed events are persisted with actor role, timestamp, action, and structured payload.
- Current status is derivable from valid events or is updated atomically with them.
- Reload/restart tests prove the event timeline survives.

### P1 test gap: tests validate mechanics, not the user promise

Current backend coverage checks persistence, adjacent transitions, invalid input, and the dangerous header catalog. Current UI coverage checks create and one forward move. Missing hard cases include cancellation, failed mutation preserving form data, stale revision conflicts, return instruction validation, approval prerequisites, malformed persisted data, evidence rendering safety, reload timeline, and keyboard/mobile decision flow.

Required next test set:
1. contract creation validation and persistence;
2. submit proposal/evidence -> review -> approve happy path;
3. review -> return requires instruction and appends an event;
4. stale revision returns 409 and preserves current state;
5. failed API response does not optimistically move a card;
6. untrusted evidence text is rendered inertly;
7. restart/reload retains contract, evidence, revision, and history.

### P2 UX gap: the board optimizes scanning before decision quality

Four equal columns and compact cards are adequate for overview, but consequential decisions need a detail workspace. On narrow screens the board becomes a horizontally scrolling set of 240px columns; this is usable for status scanning but poor for reading contracts and evidence. Do not solve this by making cards taller.

Acceptance standard:
- Selecting a card opens a keyboard-accessible detail panel with contract, proposal, evidence, timeline, and context-sensitive actions.
- Mobile uses a stage filter/list plus full-width detail rather than requiring horizontal traversal for decisions.
- Focus returns to the initiating card after close; async errors are announced.

## Recommended next vertical slice

Implement one explicit `proposal review` loop end to end:

1. Extend persisted items with `outcome`, `acceptance_criteria`, `revision`, and append-only `events`.
2. Add semantic mutation commands rather than arbitrary status PATCH: `start`, `submit_proposal`, `return`, and `approve`. Each carries `expected_revision`; return carries `instruction`; proposal carries summary plus evidence.
3. Keep dangerous-confirm middleware on every write and update the risk catalog action names/reasons.
4. Add a card detail panel. Keep the overview columns, but render explicit review actions only in the applicable state.
5. Cover the seven hard cases above, then run full Go/web gates and a real isolated-browser journey including refresh and a simulated stale second tab.

This is intentionally narrower and more valuable than adding Agent counts, activity charts, chat links, drag-and-drop, filters, or decorative telemetry. Those would recreate the failure mode of a display cockpit or polish generic project management before the collaboration protocol exists.

## Residual technical notes

- Same-status PATCH currently succeeds and rewrites `updated_at`; semantic commands should make idempotency explicit rather than inherit this behavior accidentally.
- Corrupt `workboard.json` makes the entire board unavailable. Recovery/backup behavior is worth a later robustness pass, after the decision protocol exists.
- Process-local locking is adequate for the current single-server architecture, but revision checks are still needed for multiple clients.
- No production service, secret, or unrelated process was accessed during this inspection.
