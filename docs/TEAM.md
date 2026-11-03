# Team implementation

[中文](TEAM.zh-CN.md)

The Team port targets local macOS, Linux and Windows Desktop. Its implementation
is tracked in [the delivery ledger](TEAM_PORT_STATUS.md). The current increment
provides storage, orchestration, Controller seats and a local Desktop host with
generated member/group configuration, private conversations and group chat.
Memory, schedules, credentials and complete masking boundaries remain pending.

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
credential formats are unchanged. Additive Desktop commands/events change the
contract digest and require matching Go, frontend and Electron artifacts.
The copied MIT license is retained
in `internal/team/UPSTREAM_LICENSE` and also covers the extracted roster and
host-framing packages.

## Desktop lifecycle

`desktop/internal/teamhost` owns one writer lease and serialized seat admission.
The Desktop adapter uses the existing canonical session service, workspace
registry, Controller and detached-runtime registry. It persists canonical session
routes and keeps background work independent of foreground workspace selection.
Closing a Team chat releases its display reference. Idle background seats may
be reclaimed after ten minutes; the six-seat soft cap only reclaims safe idle
seats. Pending approvals, inbox items and jobs keep their runtime.

Shutdown cancels the clock and new admission, closes every seat, waits for
Controller store release and only then gives up the Team lease. Configuration
commands compare revisions and commit bounded retry receipts atomically with
the change. Conflicts retain the caller's draft; changed request-key reuse fails.
Program approval remains at the normal workspace boundary; roster paths do not
silently approve project programs. A member's copied preset retains complete
prompt mode, runtime-context selection and the distinction between unrestricted
and an explicitly empty tool list.

The Go runtime supplies browser member identity through private grants. Electron
derives persistent partitions from that identity; different seats of one member
share login state and different members use different stores. Renderer page-open
requests ask Go for their owner and cannot choose a partition. Restored pages
retain the same partition and reject callbacks from the replaced page instance.

## Desktop Team navigation

Open **Team → Manage team** to create members and groups. Members copy the
entire selected preset, including prompt mode, runtime context and tool limits.
Edit that independent copy in the member editor. The initial model seeds new
conversations; existing sessions retain their chosen model. Names and avatars
update immediately. Static prompt/tool changes wait for an idle rebuild and
show pending status until applied.

Member permissions own every private and group seat. Restoration and chat
controls cannot widen that persistent choice. Full access uses the standard
Desktop acknowledgement, including the existing Windows Shell description.
Directory changes require idle affected seats and fork at a verified completed
turn into the new canonical workspace, preserving history. Reopen the member
from Team after moving its directory; the old binding rejects new work. Normal
workspace trust checks still apply.

Members and groups support ordering, pinning and hiding. Hiding leaves work
running. Confirmed deletion refuses active work, approvals, unconsumed exchanges
and queued rounds, and offers a stop action for the affected target. Retired
identities and logs remain readable. Retired groups appear under **Archived
history** and cannot receive messages.

Group chat uses the standard transcript, ChatSource and sole viewport writer.
Mentions store IDs, including names with spaces, and explicitly flag departed
members. History has immutable cursors in both directions and at most three
resident 32-event pages. Appends preserve cursors; replacement returns
`stale_cursor`, retaining the current reading window until explicit reload.
Reader intent defers live replacement.

Group drafts, mentions and submission identities persist in the Team namespace.
Draft CAS conflicts preserve input and require an explicit reload before
overwriting it. Quit joins admitted submissions and draft saves. A restart retry
uses the same saved submission identity. Read watermarks bind to the presented
snapshot; late acknowledgements cannot read a replacement log. Private unread
indicators reuse the existing canonical-session result sequence metric.

The browser gate uses the existing preview bridge and never fabricates member
model replies. It verifies UI/navigation separately from native-host, real-model
and installed-package acceptance.
