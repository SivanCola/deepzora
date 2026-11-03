# Team qualification / 团队验收记录

This record distinguishes source tests, native packaged tests, installed-artifact
tests and real-provider requests. No release or PR merge is performed by these
checks. / 本记录区分源码测试、原生打包测试、安装后测试及真实模型调用；验收不合并
PR，也不发布正式版本。

## Source identity / 源码身份

- Port base: `c6591b22d714ff43f17750974cb1c8078eb4ff99`.
- Refreshed main: `dfcd04d3a204f50904fdf416c6f95c150f847e5e`. The foundation
  remains mergeable; a virtual merge of the complete stack has no conflicts.
  The stack retains the approved base instead of importing unrelated changes.
- Runtime boundary candidate: `253cb7ec286c4e69f72f6d2af9a22b2ab3f3a9bb`.
- Installer qualification adds only acceptance scripts in
  `58aa56f3f7c76322635fc6bae4b801f94f89e618`; all three native installed-artifact
  checks passed. Delivery documentation is a later documentation-only commit.
- All seven fixed upstream sources and license ownership are recorded in
  [the implementation ledger](TEAM_PORT_STATUS.md).

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
