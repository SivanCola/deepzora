# Team port implementation ledger

Base: `c6591b22d714ff43f17750974cb1c8078eb4ff99` (Deepzora main).

The accepted scope is the complete seven-layer Team feature for local macOS,
Linux and Windows Desktop. CLI and remote Team entry points are excluded.
Windows follows the existing host-user shell permission contract. Members copy
preset definitions instead of following later preset edits.

## Delivery gates

| Layer | Owner | Status |
| --- | --- | --- |
| 1 | Versioned roster, exchanges, group orchestration | Implemented; focused/race/static checks passed; broad verification in progress |
| 2 | Controller seat, tools, approvals and delegation ceilings | Pending |
| 3 | Desktop runtime, lease and browser isolation | Pending |
| 4 | Desktop Team UI and generated contract | Pending |
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
- No Controller, Desktop, provider or UI entry point is enabled by this increment.

本层已实现名册、交换账本、群轮次与宿主 framing，定向测试、race、vet、repolint
通过；Windows amd64 测试二进制已交叉编译，尚无 Windows 原生运行证据。
本层没有接入 Controller、Desktop、模型或界面入口，其余六层仍待完成。
