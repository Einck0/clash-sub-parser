# Antigravity evidence-scope clarification — 2026-11-13

Scoped clarification of feature `003-probe`; no business-code change or new feature binding. This records source attribution and the precedence already declared by `probe-state-20261107.md`, not a claim of newly completed implementation or verification. Historical outcome clauses and evidence remain intact. Publication fallback/global PASS semantics are outside this clarification.

## Source attribution

The following quotes come from handoff artifacts explicitly labelled `verbatim_current_commands`; original chat records were not independently inspected:

- `/home/service/hermes/workspace/tmp/intent_package_csp_probe_continue_20261106.md`, `verbatim_current_commands`: 用户：“先继续csp的任务吧，先优化探针引擎各个方面，效率，准确性等等都要优化，然后重点看看gemini的检测是否可以在支持一个反重力可用性检测”。This asks to investigate Antigravity availability detection; it does not define an authenticated capability contract or select a verdict for anonymous reachability.
- `/home/service/hermes/workspace/tmp/intent_package_csp_probe_state_20261107.md`, `verbatim_current_commands`, repeated by `intent_package_csp_probe_resume_20261107.md`: 用户：“提示词已经改好了，现在你去改一下 CSP的任务，完善探针逻辑，包括所有的未知，以及Gemini以及反重力逻辑，有一些检测的区域与实际出口区域地址判断不符，判断为异常，但不是不可用，就是鉴于可用与不可用之间”。This supports explicit evidence-based outcomes and an abnormal intermediate state, not invented certainty about account eligibility.
- The state handoff's `discussion_context` distinguishes product-entry reachability from account/model eligibility. That paragraph is a handoff interpretation, **not a verbatim user decision**. Its `historical_reference_context` also preserves the user's objection to treating unknown as a generic bucket; it does not authorize converting unverified product eligibility to success.
- `/home/service/hermes/workspace/tmp/csp-global-pass-pi-20261112.md` contains the later global empty-group PASS correction and test-delegation requirements, but no Antigravity result-semantics correction. Do not infer an Antigravity decision from it.
- `probe-optimization-20261106.md` chose `unknown/reachable/antigravity_reachable_auth_unverified` for positive anonymous entry evidence. The later `probe-state-20261107.md` explicitly superseded that outcome with `available/reachable/network_entry/antigravity_entry_reachable`; `spec.md` STATE-R4 and `plan.md` already declare this precedence. Both verdict choices are authored engineering decisions, not located user quotations selecting one enum.
- `probe-prior-art-20261107.md` records prior official FAQ research: account-associated region, age, organization access and authenticated model use are separate from anonymous website access. These are historical retrieval claims, not fresh provider validation in this round; no anonymous account-eligibility API was established by that bounded research.

## Decision and safe interpretation

There is historical wording drift, but **no unresolved current unknown-versus-available contract conflict**: the explicit 2026-11-07 amendment already supersedes the 2026-11-06 reachability outcome. Preserve that limited positive observation rather than reverting the detector solely to match superseded text. This is a strategy interpretation of the current scoped contract and user accuracy/explicit-outcome goal; do not report it as the user expressly choosing `available` for Antigravity.

`available` means only that the named observation contract matched. For this detector it means **official anonymous product entry reachable**. Account eligibility, associated-country eligibility, login, IDE operation, API access and actual model use remain **unverified**. Unverified eligibility is not an independently successful account/model probe, and the old positive unknown rows must not be relabelled or backfilled.

Keep the existing positive tuple together: `verdict=available`, `sub_tier=reachable`, `evidence_scope=network_entry`, `reason=antigravity_entry_reachable`, explicit eligibility-unverified summary. A positive requires the official final HTTPS origin, HTTP 200, complete bounded recognized product-page evidence, and no login/challenge evidence. Generic 200 is not sufficient. A semantic change to authenticated product testing would require a separate supported method and credential/side-effect authorization; none is introduced here.

Known execution failures retain typed `error`; explicit denial/challenge/rate limiting retain `restricted`; control/budget prevention retains `skipped`; intrinsically indeterminate or historical evidence may remain `unknown`. A successful website observation does not prove all Antigravity uses succeed; a failed detection does not independently prove the entire node is unusable. Baseline health/publish eligibility remains unchanged.

## Current consumer facts and implementation instructions

Source inspection at checkout HEAD `801598aea3d5edfddd086c8a3eec4555520b0ec6` (with unrelated in-progress Gemini changes preserved):

- `internal/probe/platform/antigravity.go:CheckAntigravity` already returns the limited positive tuple and explicit unverified summary. `internal/application/probe/runner.go` invokes it independently, respects the platform selection, and keeps it in the per-platform map.
- `internal/domain/probe.go:AggregatePlatformVerdicts` intentionally preserves any-available aggregation after abnormal precedence. A group can therefore be available on positive Antigravity entry evidence alone even when siblings fail. This is scoped aggregation, **not proof of any successful authenticated AI/model call**. Keep per-platform evidence authoritative; do not silently change group aggregation in this clarification.
- `web/src/features/probes/probeTypes.ts:formatPlatformBadge` correctly labels a positive Antigravity result `入口可达`, not account/model availability. However, `buildPlatformTooltip` currently unconditionally inserts “官网入口可达” for every Antigravity verdict, including error/restricted/skipped/unknown. That is a genuine consumer overclaim, independent of the historical enum dispute.

For the assigned UI executor: make the Antigravity tooltip's headline evidence/verdict-sensitive without changing the detector, enum/schema, historical storage, aggregation, or node-health semantics. A current positive result may say “官网入口可达”; nonpositive results must use neutral scope language (for example “检测范围：官网入口连通性”) plus their actual reason/status, never assert reachability from the platform name alone. Always keep account/model eligibility explicitly unverified. Historical unknown observations may retain their own recorded reachable summary/reason; do not manufacture a positive assertion when that evidence is absent. Apply the same formatter to list/detail so they agree. Keep this a small consumer fix, not a new policy layer or credential probe.

## Deciding acceptance boundaries

These are requirements for subsequent implementation/verification, **not tests executed by this strategy investigation**:

1. Recognized complete official entry => the full positive tuple and `入口可达` label; tooltip/summary explicitly exclude account/model verification. Generic 200, login/challenge, overflow, denial, timeout and control paths never become positive because the product name is Antigravity.
2. For error/restricted/skipped/unknown with no positive reachability evidence, both list/detail tooltips must omit the factual assertion “官网入口可达” while retaining neutral detection scope, actual reason and eligibility-unverified boundary. An `available` fixture still displays only entry reachability. Preserve historical unknown rows and their recorded evidence.
3. Preserve independent Gemini-only selection, exact per-platform scope/summary API roundtrip and the existing group aggregation contract; an available group is not an account/model eligibility promise. No health-status or publication-admission changes.
4. Parent coordinates any test outline/low-tier test-writing delegation required by the user. Existing `.github/workflows/ci.yml` is the cloud runner; no server-host/local-Docker build, typecheck or test. Same-SHA cloud results, prebuilt-image isolated UI acceptance and independent review are still unverified in this investigation. No commit/push/deployment was performed by strategy.

No live proxy/provider experiment, account probe, cloud-result review or UI execution was performed here. Original chats and current official eligibility documentation were not independently re-retrieved; the decision is bounded to the attributed artifacts, explicit amendment precedence and present source consumers.
