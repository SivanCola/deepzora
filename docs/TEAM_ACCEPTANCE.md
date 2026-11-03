# Team qualification / 团队验收记录

This record distinguishes source tests, native packaged tests, installed-artifact
tests and real-provider requests. No release or PR merge is performed by these
checks. / 本记录区分源码测试、原生打包测试、安装后测试及真实模型调用；验收不合并
PR，也不发布正式版本。

## Source identity / 源码身份

- Port base: `c6591b22d714ff43f17750974cb1c8078eb4ff99`.
- Refreshed main: `dfcd04d3a204f50904fdf416c6f95c150f847e5e`. The foundation
  remains mergeable; a virtual merge of the complete stack has no conflicts.
  This is the original implementation qualification base, before review integration.
- Runtime boundary candidate: `253cb7ec286c4e69f72f6d2af9a22b2ab3f3a9bb`.
- Installer qualification adds only acceptance scripts in
  `58aa56f3f7c76322635fc6bae4b801f94f89e618`; all three native installed-artifact
  checks passed. Later descendants contain documentation and a correction to
  Windows evidence-directory binding, plus a deterministic history-search test;
  product runtime code is unchanged.
- All seven fixed upstream sources and license ownership are recorded in
  [the implementation ledger](TEAM_PORT_STATUS.md).

## Incremental review integration / 逐层审查集成

The seven PRs were reviewed against their immediate predecessors. Review fixes
also integrate main `73c695a65feaa317d56180aa1042444313e76cd1`, including its
replacement of task/fleet execution with `spawn_agents`. The package and real-model
results below identify historical candidates; they are not exact-head evidence
for these subsequent runtime corrections. Current check results and merge state
belong to each PR. / 七层均按相对前一层的增量审查，并接入上述 main 的新子 Agent
执行实现。下文安装包及真实模型结果属于注明的历史候选，不能当作本轮运行时修复
后的同提交证据；最新检查与合并状态以各 PR 为准。

- Layer 1 serializes exchange admission with creation, retains unread replies and
  dependency ancestry, and interrupts failed reply chains without pretending
  consumption. Recovery reuses the inbox identity and never reruns original work.
- Layer 2 preserves read-only ceilings through Team coordination and direct,
  capability and runner spawning; controller panics settle Team work as failure.
- Layer 3 host lifetime and browser identity checks found no new owner defect.
- Layer 4 copies the selected preset snapshot and revalidates configuration and
  session binding inside final foreground/inbox turn admission.
- Layer 5 includes every checkpointed human turn within the review prompt budget.
- Layer 6 validates metadata before writing plaintext and journals new slots for
  recovery against the committed secret book. Unknown/corrupt books block cleanup.
- Layer 7 masks image locators at provider egress while preserving inline binary
  payloads and no-hit request bytes; spawned-child usage tests cover the new API.
  Retired-slot deletion requires durable metadata before removing credential files.

对应修复覆盖：原子交办准入、未消费回复与恢复、只读多跳与子任务权限、异常结算、
预设复制及回合准入竞态、整理证据不丢整条、密钥创建失败恢复，以及图片地址脱敏、
退役值清理耐久性和新子 Agent 用量归属。测试使用可控失败和交错，不扩大权限、
预算或超时。创建日志新增独立版本化目录，不改变现有 TeamSecretBook schema；
旧数据无日志时保持可读，恢复不扫描删除未跟踪的历史凭据文件。

Review candidate `d8d43fc92ef105149fbc2657b03a864e853edaf5` passed the
[complete hosted/native qualification run](https://github.com/SivanCola/deepzora/actions/runs/41616335219).
All three native jobs passed packaged and installed Team/private/group flows,
background retention, approvals, schedules, actual POSIX/PowerShell secret
injection, persisted-output masking, restart and member Cookie isolation.
This uses the controlled fixture provider; it is not a new real-model run.
Both Go modules report zero lint issues. The local broad Desktop run recorded
1,939 top-level passes, eight platform skips and zero failures; final admission
and secret-cleanup changes also passed focused race regressions.

本轮审查候选在上述工作流通过三平台打包与安装后验收，包含真实 Shell 密钥注入、
持久化脱敏和浏览器身份隔离。模型使用受控夹具，本轮未新增真实模型调用。
本地 Desktop 广泛回归为 1,939 项通过、8 项平台跳过、零失败，最后的准入与清理
调整另有定向 race 验证。根模块与 Desktop lint 均为零问题。

## Local source checks / 本地源码检查

The complete root module passed before the final narrow boundary corrections.
Those corrections passed owning tests, lint (zero issues), repolint with the
unchanged baseline and focused race regressions. Ordinary provider serialization
remains unchanged when no secret matches. / 根模块完整测试已通过；之后的小范围边界
修复通过所属模块、lint、原预算静态检查及定向 race，并校验普通请求无命中时字节不变。

The independent Desktop module passed all tests after durable retired-slot
cleanup: 1,907 top-level passes, eight platform skips, no failed tests; its main
package took 1,377.872 seconds on this workstation. / Desktop 独立模块完整通过，
共 1,907 项通过、8 项平台跳过，无失败。

All owning secrets/provider/event race suites passed. Session race checks passed
with the large retained-heap measurement run separately without race
instrumentation (32.364 seconds); the remaining complete Session race suite
passed in 143.444 seconds. Two all-package local attempts reached their package
alarm during stress work; neither was counted as a pass or fixed by increasing
the repository timeout. Native and installed-artifact checks passed separately.

A later root CI run exposed an existing history-search test race: it required
the background refresh to remain unfinished even when the current revision was
already indexed. The test now covers an explicitly completed refresh and the
on-demand path, validates current digests/snippets, rejects stale positional
hits and verifies eventual refresh. The old assertion fails deterministically
under the completed-refresh negative control. Search runtime code is unchanged.

Earlier local secret runs exceeded the 15-second shell-exit assertion while
the Go service had already exited. A native sample showed a Chromium background
thread waiting for the macOS preferences service. Those failed attempts are not
counted as passes. After the overlapping stress work ended, the unchanged
candidate passed the complete secret flow, normal exit and restart with the
original timeout. The clean native macOS runner independently passed both
packaged and DMG-installed flows. / 较早的本机退出超时保留为失败记录；采样显示系统
偏好服务等待。未放宽超时或修改退出机制，之后相同候选和独立 macOS runner 均通过。

Frontend typechecks, production build, all 34 transcript commands, seven settings
commands, Team browser flows, the single viewport-writer contract and unchanged
UI budgets passed. Electron typecheck, 325 tests and build passed. The complete
script suite passed 250 tests with one native DMG skip; updated workflow and
packaging fixtures passed separately. / 前端、滚动、设置、团队浏览器、Electron 及脚本
检查分别记录，不把预览桥接当作真实模型或安装验收。

## Native packages / 原生安装包

Candidate `253cb7ec2` is qualified by
[manual native CI](https://github.com/SivanCola/deepzora/actions/runs/41603300381).
Linux, macOS and Windows passed. The controlled loopback fixture
checks private chat, two ordered group speakers, background retention, fresh
memory approval, scheduler provenance, real shell credential injection, masking,
persisted-data scanning, restart and native browser Cookie isolation.

Candidate `58aa56f3f` additionally qualifies the distributed DMG/DEB/Windows
installer through
[installed-artifact CI](https://github.com/SivanCola/deepzora/actions/runs/41604291425).
All three native jobs passed. macOS copies the app from a read-only mounted DMG into a
temporary installation directory. Linux installs/removes the DEB on a disposable
runner. Windows reuses clean-account checks, NSIS installation, identity and
registration checks, Team flows, uninstall and user-data retention checks.
The initial Windows helper wrote detailed reports into its temporary default
directory while the requested upload directory received the outer success
record. Its binding now preserves the requested directory, a PowerShell
regression checks that loading helpers has no side effects, and qualification
requires the detailed install/uninstall/retained-data and installed Team reports
inside the uploaded artifact. This acceptance-script correction changes no
runtime or model behavior.

| Native runner | Architecture | Package and installed flow | Secrets | Browser isolation |
| --- | --- | --- | --- | --- |
| macOS | arm64 | DMG copy and application restart passed | Passed | Passed |
| Linux | amd64 | DEB install, Team flow and package removal passed | Passed | Passed |
| Windows | amd64 | NSIS install, Team flow, uninstall and retained data passed | Passed | Passed |

Downloaded macOS evidence was read back: `identity.json` records `complete:true`
and `secretRequired:true`; the installed DMG report records `completed:true`.
The installed DMG SHA256 is
`c174b1f6405f6d1f3763f3042ce9a7b5f891b72442b58e982d90011e45951984`.

Artifacts use unpublished `v0.0.0` plus the exact source SHA and artifact hash.
They are qualification builds, not signed/public release distributions. Native
runner tests do not claim coverage of every OS version or CPU architecture.

## Real provider / 真实模型

On macOS, the downloaded `58aa56f3f` candidate passed three actual DeepSeek Flash
requests through the OpenAI protocol: one member-private reply and two ordered
group replies. The three usage records retain member/group ownership and total
22,615 tokens. Its executable SHA256 was checked against native CI evidence:
`3a570935ea2c3044a09c7d2fbc4fc6def5c08bd534cb40087b5966349199de1d`.
Both Electron and Go exited normally. / macOS 打包候选通过真实
DeepSeek Flash 私聊与双成员顺序群聊，共 3 次调用、22,615 token，用量归属及正常退出
均通过。

This bounded check used a disposable home, synthetic text, a two-minute task
limit, a USD 0.03 task budget and a 1,024-token output cap. It did not copy user
conversations or change existing user configuration. Credential material is kept
outside the repository and never uploaded with CI evidence. Linux/Windows model
flows use the explicitly identified loopback fixture; real-provider coverage is
macOS only. The earlier `253cb7ec2` candidate independently passed the same real
flow with three requests and 22,728 tokens.

## Boundaries / 边界

Local credential slots are unencrypted and readable by same-account programs.
Exact masking does not stop encoded/transformed leakage. Windows Shell runs as
the current user; file-tool checks and host approval do not imply an OS sandbox.
Old raw immutable history remains local; projected history bodies above 64 MiB
fail closed while masking is active. / 凭据未加密、同账户可读；精确值遮罩不阻止编码
变换泄漏；Windows Shell 沿用当前账户权限；旧原始对象保留，超大正文拒绝投影。
