# Workboard inspection 04 — auditability and collaboration semantics

Date: 2026-08-09
Role: human approver, compliance reviewer, and concurrent operator
Baseline: `81e1b2d feat(workboard): add revisioned decision workflow`

## Verdict

The revisioned command API is a material improvement over generic Kanban: contracts, explicit commands, conflict detection, return guidance, evidence, and a decision surface all work. Focused API and UI tests pass. However, the current slice is not yet a trustworthy action-collaboration record. It can show the latest proposal, but it cannot prove what proposal and evidence a human previously returned, and its actor labels claim agent activity even when the human typed every action in the browser.

This is above the functional pass line, but below the product bar implied by evidence-based approval and acceptance.

## Inspection evidence

### 1. Blocking — resubmission destroys the proposal under review

`submit_proposal` overwrites `item.Proposal`. The appended event contains only action, actor, note, revision, and time; it has no proposal or evidence snapshot.

A temporary black-box API test exercised:

1. create;
2. start;
3. submit proposal A with evidence A;
4. return with direction;
5. submit proposal B with evidence B.

The test failed as intended: the final item contained proposal B, while no event retained proposal A or evidence A. The temporary test file was removed after execution and the worktree returned to only the pre-existing `CHANGELOG.md` modification.

Impact: after a return/resubmit cycle, an approver cannot reconstruct what was rejected, compare revisions, or audit whether feedback was addressed. The visible timeline overstates auditability.

Required improvement: persist immutable proposal revisions (summary, evidence, submission revision/time/source), and have decision events reference the proposal revision they decided. Show proposal history or a revision comparison in the detail surface.

### 2. Blocking — actor attribution is not truthful

The public Workboard UI lets a human click `Start work` and type both the proposal and evidence. The server nevertheless hard-codes `start` and `submit_proposal` events as actor `agent`. There is no authenticated or request-level source field and no Agent/Goal execution integration that produced these events.

Impact: the record makes an unsupported claim about who acted. This is worse than omitting actor data because evidence and approvals depend on provenance.

Required improvement: use an explicit, validated source such as `human`, `agent`, or `system`, supplied by trusted integration context rather than inferred from action name. Until a real Agent connector exists, browser-originated commands must be recorded as human. The UI should visibly distinguish manual proposals from Agent-produced proposals.

### 3. Blocking — “approve proposal” and “accept result” are collapsed

The current state transition is `submit_proposal: active -> review`, then `approve: review -> done`. The proposal text describes a plan, while evidence may describe completed work. Therefore the same approval ambiguously means either “the plan is authorized” or “the result satisfies acceptance criteria.” There is no execution state after plan approval, no per-criterion verification, and no acceptance record binding a decision to evidence.

Impact: a human can mark an unexecuted plan done, or cannot approve a high-risk plan before execution without falsely completing the item. This breaks the objective’s separation of proposal, human approval, action, evidence, and acceptance.

Required improvement: choose and encode distinct semantics. Recommended minimum flow:

`backlog -> proposed -> approved_for_action -> executing -> acceptance_review -> accepted`

A smaller compatible variant may retain four columns but must use separate commands and records for `approve_plan` and `accept_result`. Acceptance must bind to a specific proposal/result revision and capture criterion-level pass/fail or explicit exceptions.

## Important usability gaps

### 4. Evidence input is artificially single-row

The API accepts 1–12 evidence items, but the Workboard UI exposes exactly one label/detail pair. Existing cards can display multiple evidence entries only when another client created them.

Improve with add/remove evidence rows, stable row keys, and tests for two or more evidence entries. Evidence should support reference types (URL, file/path, test result, note) later, but multiple structured rows are the immediate need.

### 5. The decision dialog is visually modal but incomplete as a keyboard modal

The detail panel has `role="dialog"` and `aria-modal="true"`, but inspection found no Escape handler, focus trap, initial focus placement, or focus restoration to the opening card. Background controls remain keyboard reachable.

Improve with a tested focus lifecycle and Escape close behavior. Also repeat the prior geometry check in a fresh isolated browser session because the post-fix overlay dimensions were not re-measured after the automation tab stalled.

### 6. Conflict recovery stops at an error banner

A stale revision correctly returns 409 and leaves the dialog open, which prevents silent overwrite. The user receives no direct reload/compare action and may continue editing against a stale proposal.

Improve by preserving the draft, fetching the current item on 409, and showing a compact comparison with “use latest and reapply” rather than only an alert.

## Test quality assessment

Current focused tests cover the happy command path, required evidence, missing confirmation, invalid state commands, stale revision rejection, legacy normalization, create payload, proposal submission, and inline conflict display. Missing high-value cases:

- immutable proposal/evidence history across multiple returns;
- truthful source/actor attribution;
- plan approval distinct from result acceptance;
- acceptance bound to a proposal revision and criteria;
- multiple evidence rows in the UI;
- conflicting submit/return requests under race;
- whitespace and upper-bound validation for proposal, note, evidence label/detail;
- keyboard modal lifecycle and Escape;
- conflict reload while preserving the local draft.

## Next improvement slice

Prioritize correctness over dashboard polish:

1. introduce immutable `proposal_revisions` and decision references with legacy normalization;
2. add truthful command source attribution, defaulting current browser actions to human;
3. split plan approval from result acceptance and bind acceptance to criteria/evidence;
4. expose multiple evidence rows and proposal history in the detail panel;
5. add focused API/race/UI tests, then run full Go and web gates;
6. verify the complete return/resubmit/approve/accept cycle in a fresh isolated browser service and record final overlay geometry.

Do not spend the next slice on charts, counts, cosmetic dashboard metrics, or chat-like text streams. The highest-value improvement is making each action and decision provable.
