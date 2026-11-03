# Team port implementation ledger

Base: `c6591b22d714ff43f17750974cb1c8078eb4ff99` (Deepzora main).

The accepted scope is the complete seven-layer Team feature for local macOS,
Linux and Windows Desktop. CLI and remote Team entry points are excluded.
Windows follows the existing host-user shell permission contract. Members copy
preset definitions instead of following later preset edits.

## Delivery gates

| Layer | Owner | Status |
| --- | --- | --- |
| 1 | Versioned roster, exchanges, group orchestration | Implemented; focused/race/static checks and full root Go suite passed; PR #146 |
| 2 | Controller seat, tools, approvals and delegation ceilings | Implemented; full root suite and hosted CI/CodeQL passed; PR #147 |
| 3 | Desktop runtime, lease and browser isolation | Implemented; full root/Desktop suites and macOS native browser fixture passed; PR #148 |
| 4 | Desktop Team UI and generated contract | Implemented; focused/race, production build, 246 settings viewport cases, Team browser flow and transcript checks passed; broad Go validation running |
| 5 | Memory, opt-in review and schedules | Pending |
| 6 | Private credential slots and shell injection | Pending |
| 7 | Outbound/stream masking, usage and native acceptance | Pending |

No layer status implies completion of a later layer. Secret delivery must remain
unavailable until its complete masking boundaries pass. Hosted checks, local
checks, native platform acceptance and publication are separate evidence.

## Source snapshots

The upstream repository is `SivanCola/DeepSeek-Reasonix`, submitted through
`esengine/DeepSeek-Reasonix` PRs #14636, #14637, #14638, #14639, #14640,
#14641 and #14623, in that order. Fixed heads:

1. `1c8f9818013e30de612fd794b6f2b9039bd854f9`
2. `c54659170682ea1011cfde895b2bb86ca9e7aef6`
3. `58d89be95d39b77227c736fb8a1ff0058218ef41`
4. `2fe0db5d6f8a389a0239f3d208e578a65c1cd29c`
5. `ca38fcc4311d7c570d06a319c159ddbd61eac35a`
6. `ac40a8944f5771f190c4a090da4dcb9af3b2cfd8`
7. `d5e43581e2ea6f4845e46fb29968cc1b2cb23b3a`

Extract mechanisms and their regressions into Deepzora owners; do not import
Studio HTTP plumbing, terminal outbox, unrelated fixes or release infrastructure.

## Foundation evidence

- Extracted the fixed part-1 roster, exchange, group-round and host-frame owners
  together with their regressions. Kept the upstream MIT attribution.
- Adapted imports, canonical permission/ceiling values, identity-aware locks,
  strict atomic replacement and portable path fixtures to Deepzora.
- Added a real child-process lease regression, canonical-mode rejection and
  stale-revision regression.
- Found and reproduced an upstream idempotency gap: the same request key could
  change recipient, parent, group, reply destination or execution ceiling while
  retaining the same text. All five negative controls failed before the repair;
  the store now rejects those mismatches.
- Focused package tests, race tests, vet and repolint passed. Windows amd64 test
  binaries compiled; Windows native execution has not been performed.
- The full root `go test ./...` completed successfully. Linux amd64 roster and
  service test binaries also compiled. Hosted CI remains separate evidence.
- No Controller, Desktop, provider or UI entry point is enabled by this increment.

本层已实现名册、交换账本、群轮次与宿主 framing，定向测试、race、vet、repolint
通过；Windows amd64 测试二进制已交叉编译，尚无 Windows 原生运行证据。
第一层单独不启用 Controller 或桌面入口；后续层状态见上表。

## Seat/controller increment

- Added an optional host-owned boot seat and a fixed Team tool set, preserved
  across controller rebuild. Ordinary boots do not register Team tools.
- Identity and roster ride the turn tail; provider-boundary tests compare the
  unchanged ordinary system prefix and stable Team schemas across turns.
- Team input uses the existing durable inbox and explicit host message origin.
  Admission validates pending work; replay of interrupted exchanges is refused.
  The admitted envelope owns immutable turn metadata instead of rereading a
  mutable sidecar to discover authority during execution.
- Scoped execution ceilings preserve the user's persistent permission and also
  reach child execution and resolved proxy targets. A compiler-overlay negative
  control executes the forbidden writer when enforcement is removed.
- Fresh Team configuration approvals cannot reuse session grants or Full access.
  Writes compare the reviewed roster revision and recheck target authority.
- Queue prose editors cannot rewrite a host-authored Team delivery. Task children
  cannot borrow their parent's Team identity. Coordination is classified as host
  state, not a work-product mutation.
- Focused regressions and race checks passed. The broad Controller suite exposed
  an approved-plan fallback regression; the repair limits the restriction to an
  actual active Team ceiling. The complete Controller, Agent, boot, Team,
  builtin, host-frame and evidence suites passed after that repair.

第二层已接入可选席位、固定工具目录、持久 inbox、宿主来源、回合结算和临时执行上限，
并验证系统前缀稳定、只读代理执行、陈旧审批拒绝及已结算任务不重放。普通已批准计划的
回归已修复，全套相关包已通过。桌面 TeamHost 与界面尚未接入。

## Desktop host increment

- Added the local lease/recovery fence, serialized seat creation, idle admission,
  background retention and shutdown join. Recovery errors keep the host sealed
  and retain the lease until repaired or all runtime teardown completes.
- Canonical sessions use the existing workspace creation protocol and the
  Controller/inbox lifecycle. Team routes cannot reopen as ordinary sessions
  after eviction. Background opens preserve foreground selection. Runtime
  teardown joins Controller.Closed outside the global admission lock.
- Added generated query/configuration/private-open commands and Team change
  events. Revision conflict and request-key replay are explicit results.
- Copied presets retain complete prompt/runtime-context/tool-list semantics.
  Provider-boundary tests prove source replacement cannot change the copy.
- Browser storage identity comes from Go grants; different member partitions,
  same-member seats and restoration passed the macOS native Cookie fixture.
- That native fixture reproduced a late-destroyed-event bug: an old page could
  delete a replacement using the same logical id. The negative regression failed
  before the instance fence; the full native fixture passed after repair,
  including zero remaining WebContents.
- Adapter/host race regressions, all 325 Electron tests, focused browser tests,
  Electron/frontend typechecks, Desktop lint and repolint
  passed. Broad root validation initially failed only the generated inventory;
  regeneration and its owning test passed, followed by a successful complete
  root rerun. The complete Desktop Go suite also passed (main package 730.880s).
  Generated Team command bridge declarations and the frontend production build
  passed after repairing an omitted declaration.
- Linux/Windows native Team acceptance and packaged acceptance are still pending.
  No secret feature is exposed by this layer.

第三层已接入规范会话、后台生命周期、类型化配置命令和成员浏览器分区，真实 Controller
与 inbox 测试及 macOS 原生 Cookie/恢复/清理测试通过。完整根模块与 Desktop 测试通过；
Linux、Windows 原生功能验收尚未完成，团队界面及第五至七层尚待实施。

## Desktop UI increment

- Added Team navigation, independent preset/member editing, group management,
  ordering, visibility, confirmed retirement and retained historical identity.
- Canonical private conversations and group transcripts reuse existing owners.
  Structured mentions use IDs. Group history has bidirectional immutable
  snapshots and three bounded pages; stale results preserve the reading window.
- Configuration commits fence new exchange/round admission. Directory changes
  preserve history through canonical forks; old bindings reject delivery.
  Restored seats use the member-owned persistent permission.
- Independent draft CAS, durable submission IDs and snapshot read watermarks
  cover delayed saves, stale clears, shutdown joins, restart recovery, replacement
  logs, busy retirement and idempotent retry receipts.
- Frontend typecheck/build, app-layer/scroll-writer gates and unchanged resource
  budgets passed. Settings browser verification passed 246 cases in English and
  Chinese at 1600/900/400px. Team browser verification passed creation, ordering,
  names with spaces, switching, retained drafts and one accepted event.
- These browser checks use the preview bridge. They do not replace real-model
  or Linux/Windows native/package acceptance. Layers 5–7 remain pending.
