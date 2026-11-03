# Team implementation

[中文](TEAM.zh-CN.md)

The Team port targets local macOS, Linux and Windows Desktop. Its implementation
is tracked in [the delivery ledger](TEAM_PORT_STATUS.md). The current increment
provides storage, orchestration and an optional Controller/boot seat adapter.
The Desktop host and interface are not enabled yet.

## Ownership

`internal/teamroster` owns versioned roster validation, revision-checked writes,
exchange records, group logs and round state. Writes use Deepzora's identity-aware
cross-process locks and strict atomic replacement. The host must acquire the
writer lease before driving the service; a plain Store is a persistence primitive,
not an authorization boundary.

`internal/team` owns agent authorization, bounded handoffs, group speaker order,
timers and recovery. It reaches sessions only through `Panes`. `Pane.Deliver`
must acknowledge durable inbox storage before returning success. The controller
must report the actual outcome and replies consumed through `Settlement`.

`internal/hostframe` makes untrusted text inert with respect to host-owned tags.
It does not grant execution authority. Agent instructions and messages remain
data subject to the receiving session's execution permissions.

## Persistence and compatibility

Roster schema 1 is new Deepzora-only state. Only the canonical permission values
`read-only`, `workspace-write` and `danger-full-access` are accepted; an omitted
permission is left for the desktop owner to resolve. No Reasonix data is imported.
Newer schemas refuse writes; corrupt records are preserved and reported.
Interrupted trailing JSONL entries are ignored and cut before the next append.

A request ID can retry only the same sender, recipient, parent, group, reply
destination, execution ceiling and message. A mismatching retry is rejected.
On restart, unfinished exchanges and rounds are interrupted before sessions resume;
the eventual controller adapter must refuse queued work that has already settled.

The writer lease coordinates cooperating processes on all three platforms. It
does not isolate the store from programs running as the same operating-system
user. This is especially relevant to Windows shells, which follow Deepzora's
existing host-user execution contract.

## Validation

Run `go test ./internal/teamroster ./internal/team ./internal/hostframe` and the
same packages with `-race` where supported. The regression suite covers revision
conflicts, corruption, future schemas, actual child-process lease contention,
canonical permissions, idempotency, handoff cycles, consumed replies, group
timers, approval contracts and restart recovery. Windows cross-compilation is
only compilation evidence; native runtime acceptance remains a separate gate.

Ordinary boots keep their existing provider request and tool registry. Team
boots add the same Team tools and append current identity at the turn tail.
Incoming Team work has host provenance and a scoped execution ceiling; it cannot
rewrite the member's persistent permission. Fresh configuration approvals compare
the reviewed roster revision before committing. Existing session, preset,
credential and desktop RPC formats are unchanged. The copied MIT license is retained
in `internal/team/UPSTREAM_LICENSE` and also covers the extracted roster and
host-framing packages.
