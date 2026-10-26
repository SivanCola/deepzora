# App session ownership

[简体中文](APP_SESSION_OWNERSHIP.zh-CN.md)

Display identity, ordered snapshots, paging and recovery are described in
[Transcript projection](TRANSCRIPT_PROJECTION.md).

Session actions capture their source when invoked. A later tab change cannot
redirect a pending send, cancel, approval, model update, or navigation completion
to the newly selected session. Layout-committed command registrations publish
authority; replacement generations and unmount revoke old continuations.
Background cancellation resolves the canonical controller target rather than a
UI tab identifier. Missing or replaced targets produce a stale outcome.

Subscription scopes revoke queued deliveries before releasing registrations.
Terminal output uses reference-counted leases so an old cleanup cannot release
a newer subscriber. AppRuntime wires these owners to AppRuntimeView. App.tsx is a small composition
entry; the view receives committed commands and presentation data without
creating a second session authority.

## Local readable history and runtime readiness

Local navigation starts a bounded canonical history read after publishing the
target tab identity, independently of controller activation. A readable history
window can appear while runtime setup is still pending. Draft input stays mounted
and editable, while send/control actions retain their runtime readiness fence.
Runtime failure preserves history that already arrived; it cannot turn an
unfinished history read into a successful empty transcript.

Navigation detaches renderer subscriptions and retains history under the existing
store budgets; explicit close evicts it. Lazy bodies can hand off once to a newer
generation of the same resident owner. Eviction or close/reopen ends the old read.
Retained residency alone does not prove a navigation cache hit: canonical history
generations and legacy tab fingerprints are different identity domains, and a new
tab binding needs explicit ownership reconciliation. Browser-mock paint timings
do not establish native backend latency or cross-binding cache reuse.

## Verification

`pnpm test:app-lifecycle` exercises source capture, committed publication,
supersession, A-to-B-to-A navigation, canonical background cancellation,
unmount, subscription disposal, and negative memory-protocol fixtures.
`pnpm test:app-browser` replays real navigation, send/Stop,
three layouts, and Composer/Workspace DOM identity. `pnpm test:all` discovers
the remaining frontend regression suites.

## Independent memory screening

The App memory workflow is a manual diagnostic. It builds the selected commit
once and runs the full three-process protocol: each process executes 128 full,
128 windowed, 128 safety, and 512 mixed trips. Ordinary PR and main CI keep
functional App lifecycle and Transcript regressions without repeating the soak.

The aggregate still requires every checkpoint and heap snapshot, distinct shard
identities, the same workflow attempt, source/build hashes, Node/platform/
architecture, fixture configuration, browser version and declared profile.
Missing, cancelled, mismatched, or failing shards cannot produce a passing result.

A `SHARD_PASS` is only one complete process. Aggregate `PASS` is automated
screening, not a whole-App memory-leak proof: heap-retainer analysis and a
mainline control comparison remain separate attribution work. Reports preserve
that pending status. PR-head evidence also does not replace integration and
native checks against the current target branch.
