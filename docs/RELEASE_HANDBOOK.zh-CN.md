# Deepzora 中文发布手册

简体中文 | [English](RELEASE_HANDBOOK.md)

适用仓库：`SivanCola/deepzora`。正式版统一从受保护的 `main` 准备和发布。
本手册覆盖 CLI、npm、桌面端、Homebrew、R2 下载及中英文官网更新；以 `0.0.9`
为示例。每次操作请替换为真实版本，保留候选 ID、源码 SHA 和运行链接。

日常发版需要人工完成：审核并合并代码与中英发布说明、确认候选安装包、批准正式发布。
Windows Certum 签名、macOS 签名和 Apple 公证在准备候选时无人值守执行，不需要
逐次输入手机验证码或登录 Apple。公开渠道验收、R2 同步校验和官网部署也自动执行。
本手册是后续正式发版的操作入口；签名异常先查下文的“需要人工维护的情况”。

```mermaid
flowchart TD
  A[审核代码和发布说明，合入 main] --> B[自动构建、签名、公证、原生验收并封存]
  B --> C[人工确认候选，发起正式发布]
  C --> D[维护者批准 release 环境]
  D --> E[发布 CLI、npm、Desktop 和 Homebrew]
  E --> F[公开渠道 postflight 验收]
  F --> G[R2 同步并完整回读校验]
  G --> H[推进官网版本指针，触发 Pages]
  H --> I[中英文生产页面验收通过]
  I --> J[旧版 Windows 客户端公开更新通道验收]
```

## 一、发版前检查

### 1. 账号、工具和源码

- 使用具备仓库操作权限的 GitHub 账号；正式发布还需要 `release` 环境的审批权限。
  维护者名单以 GitHub 仓库 Settings → Environments → release 为准。
- 本机安装 Git、GitHub CLI (`gh`)、`jq`、Bash，以及仓库 `.node-version`
  指定的 Node.js。普通发版通过 Actions 执行，不需要在本机保存 R2 或签名私钥。
- 发布代码和发布说明已通过 PR 审核并合并至 `main`，相应 CI 通过。
- 版本使用尚未发布的稳定版本号，例如 `0.0.9`，输入工作流时不带 `v`。
  不手动创建 `v0.0.9`、`npm-v0.0.9`、`desktop-v0.0.9` 标签；由发布流程原子创建。
- 确认没有同一候选的另一个发布正在运行或等待审批，避免重复派发。

在仓库根目录检查身份和本地状态：

```sh
git status --short --branch
git fetch origin main --tags
git log -1 --format='%H %s' origin/main
gh api user --jq .login
node --version
jq --version
```

有本地修改时先处理好，不直接切分支覆盖它们。后续本地脚本应来自已同步的正式
源码检出；远端工作流始终指定 `--ref main`，不会因为本地位于功能分支而发布该分支。

准备候选前记录准确源码，并确认其对应的 push CI 成功：

```sh
release_source_sha="$(git rev-parse origin/main)"
RELEASE_CI_WAIT_SECONDS=0 bash scripts/verify-release-push-ci.sh "$release_source_sha"
```

若 CI 尚在运行或失败，先等待或修复再继续；候选准备时还会独立执行此门禁。

### 2. 签名、发布凭据和云端设置

下表用于首次配置、凭据轮换或发版预检失败时核对。只检查名称和有效性，不把值写入
文档、日志、截图、PR 或普通仓库文件。配置入口为 GitHub Settings → Secrets and
variables → Actions；环境级配置同时核对相应 Environment。

| 用途 | Actions Secrets | Actions Variables / 外部要求 |
| --- | --- | --- |
| macOS 签名、公证 | `APPLE_CERT_P12`、`APPLE_CERT_PASSWORD`、`APPLE_API_KEY_P8`、`APPLE_API_KEY_ID`、`APPLE_API_ISSUER_ID` | 证书、API 凭据、团队会员资格和开发者协议有效 |
| Windows Authenticode | `CERTUM_USERNAME`、`CERTUM_OTP_URI`、`CERTUM_KEY_ID` | 签名服务和证书有效 |
| 安装包完整性签名 | `MINISIGN_PRIVATE_KEY`、`MINISIGN_PASSWORD` | `MINISIGN_PUBLIC_KEY` |
| 正式标签 | `RELEASE_TAG_TOKEN` | `RELEASE_TAG_ACTOR` 与凭据身份一致，且获标签规则授权 |
| npm / Homebrew | `NPM_TOKEN`、`HOMEBREW_TAP_TOKEN` | 对目标包、tap 有发布权限 |
| R2 | `R2_ACCESS_KEY_ID`、`R2_SECRET_ACCESS_KEY`、`R2_ACCOUNT_ID`、`R2_BUCKET` | `ENABLE_R2_DOWNLOADS=true`；`R2_PUBLIC_BASE=https://downloads.deepzora.app` |
| 官网 | `CLOUDFLARE_PAGES_DEPLOY_HOOK` | Pages 项目 `deepzora-website` 的钩子指向 `main` |

R2 凭据仅允许读写 `deepzora-releases`。不要用历史变量 `ENABLE_R2_MIRROR`
代替当前安装包镜像开关 `ENABLE_R2_DOWNLOADS`。Pages 保留原生 GitHub 连接，生产分支
为 `main`、根目录 `website`、命令 `npm run check`、输出目录 `dist`；详见
[官网配置](../website/README.md)和 [R2 配置](R2_DOWNLOAD_MIRROR.md)。

**Windows 按目标版本核对签名策略：**当前
[签名策略](../scripts/release-windows-policy.mjs)明确允许稳定版 `0.0.1`、`0.0.2`、
`0.0.3`、`0.0.4`、`0.0.5` 和 `0.0.6` 使用维护者批准的未签名例外。Windows `0.0.6` 不以配置 Certum 为前提，
但仍必须通过 Minisign 完整性签名、来源验证和最终安装包的原生验收；发布说明须保留
未知发布者与 SmartScreen 风险。该例外不适用于预发布版或 `0.0.7` 等未列出的版本。

每次准备候选前，以已审核源码中的策略和实时配置为准，可运行
`node scripts/release-windows-policy.mjs 0.0.9 stable` 核对目标模式（替换为目标版本）。返回 `certum`
时必须配置全部三个 `CERTUM_*` Secret 及有效签名服务，否则任务会报
`Certum signing configuration is incomplete for this Windows version`。
凭据缺失不会自动降级；新增例外或更换签名方案需要独立修改并审核发布策略。

### 3. 无人值守签名的执行与验收

- **Windows**：Actions 使用 `CERTUM_OTP_URI` 自动生成登录验证码，在 GitHub 托管
  runner 上连接 SimplySign 云证书，签内部 PE 载荷、重打包并签最终安装包，验证预期
  签名者、信任链和时间戳，再生成 Minisign 完整性签名。证书私钥保留在 Certum 云端。
  x64、ARM64 最终安装包均须通过原生验收；登录会话在失败时也会清理。
- **macOS**：Actions 在临时 keychain 导入 Developer ID 证书，使用 API key 提交
  `.app` 和 `.dmg` 公证。两者均须返回 `Accepted`，完成 `stapler staple`、
  `stapler validate` 和 Gatekeeper 验证；Universal DMG 另在 Intel runner 验收。
  公证诊断保存在 `apple-notarization-<run>-<attempt>-<target>` artifact。
- **正式发布**：候选通过并封存后，发布工作流验证来源、文件字节与验收证据，再请求
  一次 `release` 环境审批。审批控制公开发布，不要求逐个签名操作人工登录。
  独立 Certum smoke test 自身也需要 `release` 审批，但不公开发布产品。

Secret 名称存在或格式校验通过不代表签名服务可用。必须以本次实际签名、公证、
原生验收及封存成功为准；不能沿用历史成功记录替代当前候选的证据。

### 4. 需要人工维护的情况

| 情况或症状 | 人工处理与恢复验收 |
| --- | --- |
| SimplySign 登录失败、验证码失效，或手机重新激活／绑定 | 先核对账号和现有配置。确需重新绑定时须获账号所有者批准，由其完成手机激活；将新的完整 `otpauth://totp/…` URI 安全更新到 `CERTUM_OTP_URI`，不能使用一次性的六位验证码。新绑定后旧 CI 配置可能失效，必须重新运行实际签名测试。不要为排障擅自重置手机 |
| Certum 证书到期、续期、撤销或证书选择变化 | 由证书持有人处理服务和证书，核对 `CERTUM_KEY_ID` 为目标证书的 40 位 SHA-1 thumbprint，必要时更新 Secret，再验收 x64/ARM64 签名者、信任链及时间戳 |
| Apple 返回 `403: A required agreement is missing or has expired` | 团队 Account Holder 登录 [Apple Developer](https://developer.apple.com/account/)，确认正确团队并阅读、接受待处理协议；这是账户法律操作，Actions 不代签。确认接受日期和提示消失后，重试公证。当前发布团队为 `2J3X86486S` |
| Apple 会员资格、Developer ID 证书到期／撤销，或 API key 失效 | 由账户持有人处理会员资格，授权维护者更新证书或 API 凭据。`APPLE_CERT_P12` 是含私钥的 P12 的 Base64，`APPLE_API_KEY_P8` 是 P8 的 Base64；核对密码、Key ID、Issuer ID 和所属团队，再用候选或隔离演练验证应用、DMG 公证及票据校验 |
| SimplySign 安装器、登录控件或签名契约变化 | 审核并修复固定版本、摘要、登录适配器或 `.github/release-signing.yml`；运行回归和实际签名测试。独立恢复的指纹维护见 [Certum 说明](CERTUM_SIGNING.md)，不手写成功回执或跳过校验 |
| Minisign 密钥丢失、轮换或疑似泄露 | 按 [安全政策](../SECURITY.md)处理。旧客户端信任内嵌公钥，不能只换 Secret 就假定仍可升级；先设计并验收信任迁移，保留旧版更新兼容 |
| GitHub 审批、标签令牌、npm/Homebrew、R2 或 Pages 凭据／权限变化 | 按上表核对对应 Secret、变量和授权，修复后从失败阶段恢复；不要重发已公开的同版本文件 |

二维码、TOTP URI、证书私钥和令牌只通过批准的安全渠道保存及更新 Actions Secrets，
不进入聊天、普通文件、截图或日志。手机激活时应保留获授权的原始配置加密备份；
手机一次性验证码不能用来恢复 CI 绑定。日常发版不需要执行手机重绑、Apple 网页登录或
凭据轮换；只有维护事件触发时才处理。

Certum 凭据或登录链路维护后，运行不发布产品的实际签名测试：

```sh
gh workflow run certum-signing-smoke.yml --repo SivanCola/deepzora --ref main \
  -f client_probe_only=false
gh run list --repo SivanCola/deepzora --workflow certum-signing-smoke.yml --branch main --limit 5
```

在对应运行批准 `release` 环境，等待两个架构的探针、签名者、信任链及时间戳检查
全部成功。Apple 维护后使用下文的候选准备或第七节隔离演练验证，无需发布来试签。

## 二、准备并审核中英发布说明

在 [release-notes/releases.json](../release-notes/releases.json) 顶部新增目标版本记录。
参考已有记录填写完整结构，不只修改版本号：

| 字段 | `0.0.9` 示例或要求 |
| --- | --- |
| `version`、`releaseId`、`baseVersion` | 均为 `0.0.9` |
| `previousRelease` | 上一个正式版本，例如 `0.0.8` |
| `date` | 实际计划发布日期，`YYYY-MM-DD` |
| `channel` / `status` | `stable` / `reviewed`；审核完成后再设置 `reviewed` |
| `builds` | `cli: v0.0.9`、`desktop: v0.0.9`、`npm: 0.0.9` |
| 内容 | 中英标题、摘要、变更、升级提醒、已知风险、适用产品和关联 PR/issue |
| 链接 | 对应版本的下载、比较和文档链接，按已有结构填写 |

`refs` 会用于生成贡献者署名。签名状态、隐私和代码签名政策必须与真实安装包一致。
验证结构并预览两种语言：

```sh
release_version=0.0.9
node scripts/release-notes.mjs validate
node scripts/release-notes.mjs render --version "$release_version" --lang zh --output /tmp/deepzora-release-notes.zh.md
node scripts/release-notes.mjs render --version "$release_version" --lang en --output /tmp/deepzora-release-notes.en.md
```

打开生成文件人工审核后，经 PR 合并至 `main`。渲染时使用 `GH_TOKEN`，未设置时会
尝试 `gh auth token`；账号需能读取相关 issue/PR。失败时先修复鉴权、限流或关联编号，
不要发布缺少署名的半成品说明。

审核应覆盖最终计划发布的所有产品改动及已知问题，不能仅凭 `status: reviewed`
或结构校验通过判断说明已完整。发布日期应与本次发布计划一致。

## 三、准备候选安装包

`release-notes/releases.json` 的变更推送到 `main` 后会自动触发
[Prepare release candidate](https://github.com/SivanCola/deepzora/actions/workflows/release-candidate.yml)，
使用目录中第一个版本。先查看是否已有对应运行：

```sh
gh run list --repo SivanCola/deepzora --workflow release-candidate.yml --branch main --limit 10
```

只有没有合适运行，或修复问题后需要重新准备时，才手动触发：

```sh
release_version=0.0.9
gh workflow run release-candidate.yml --repo SivanCola/deepzora --ref main \
  -f version="$release_version" -f rehearsal=false
```

手动运行采用触发时的 `main` 源码；自动运行采用触发它的推送 SHA。任务会检查审核
完成的 Stable 说明、源码 CI、签名配置，构建 CLI/npm 与各平台桌面包，执行最终安装包
的原生验收、生成文件证明并封存。已有有效的相同候选可以复用；该阶段不公开发布。

在 Actions 页面等待候选构建和封存全部成功，查看 **Summary → Candidate ready**，
复制完整候选 ID。CLI 跟进方式如下；`RUN_ID` 必须替换为列表中选定的真实数字：

```sh
gh run view RUN_ID --repo SivanCola/deepzora
gh run watch RUN_ID --repo SivanCola/deepzora --exit-status
```

候选 ID 格式是 `v0.0.9-<12位源码标识>-<12位目录摘要>`。原样复制，不按版本号猜测，
也不要拿 rehearsal 候选发版。记录 Candidate input 中的 Product / Control SHA，
确认源码包含计划发布的修改。

若准备候选后 `main` 又合入计划随本版发布的代码或说明，旧候选不会自动更新。
重新核对最终源码的 CI，并从该源码准备新候选；不要批准缺少这些修改的旧候选。
仅修改其他文件的后续推送不会触发候选自动构建，必要时使用上面的手动触发命令。
发布前记录最终候选源码 SHA，与计划发布范围逐项核对。

从该运行的 Artifacts 获取封存安装包，按发布范围检查安装、启动、版本号、主要功能
和升级路径。人工发现问题时停止发布，修复后合入 `main` 并准备新候选。候选封存后
再修改代码或说明，不会改变这份候选的文件。

候选 payload 保留 **30 天**，record 与 evidence 保留 **90 天**。记录还在不代表
安装包仍可用；尚未发布而 payload 过期时，需要重新准备并验收。

## 四、发起正式发布并批准

确认候选通过后，在仓库根目录设置真实候选 ID，再发起发布：

```sh
# 将引号中的说明文字替换为 Summary 复制的完整候选 ID。
candidate_id='粘贴完整候选ID'
./scripts/release-stable.sh "$candidate_id" publish
```

脚本只派发，不代表发布完成。打开
[Publish release candidate](https://github.com/SivanCola/deepzora/actions/workflows/release-promote.yml)，
核对运行标题中的候选 ID 和操作是 `publish`。预检验证封存文件、来源、说明和标签
权限后，进入 `release` 环境审批。

由有权限的维护者在 GitHub **Review deployments → release → Approve and deploy**
批准；或在交互式终端执行以下命令，按提示输入同一完整候选 ID：

```sh
./scripts/release-approve.sh "$candidate_id"
```

两种审批方式任选一种。脚本会等待预检；超过等待窗口时先看工作流状态，再重跑
审批脚本，不重复派发发布。提示没有活动发布时，先检查上一次运行是否已失败或完成。
审批后 GitHub 开始公开发布；不要删除标签或重新构建同版本安装包来修复中断。

## 五、等待自动发布、R2 与官网验收

| 阶段 | 成功含义 |
| --- | --- |
| `activate` | 同一源码的三个正式标签已原子创建或验证 |
| `cli`、`npm`、`desktop` | 使用封存文件发布相应产品及 Homebrew |
| `postflight` | 标签、GitHub Releases、npm、Stable 清单和 Homebrew 的公开状态通过核验 |
| `mirror` 中的 `Sync and read back every release object` | 官方源文件及全部 R2 对象的大小与 SHA-256 校验通过 |
| `Save verified mirror inventory` | 保存本次校验报告 |
| `Deploy and verify the official website` | Pages 接受重建，且新的生产版本记录、中英文页面和安装包链接通过检查 |
| `legacy-windows-client` / `verify original client against public update channel` | v0.0.8 起的当前公开版本：原始 v0.0.7 更新器在原生 x64、ARM64 上完成清单校验、安装包路由、下载、Minisign、缓存和 Authenticode／时间戳验证 |

发布任务内的镜像是可复用工作流，通常显示在同一次发布运行里；单独恢复镜像时才
有独立的 `Sync release downloads to R2` 运行。查看当前发布的真实 `RUN_ID`：

```sh
gh run list --repo SivanCola/deepzora --workflow release-promote.yml --branch main --limit 10
gh run watch RUN_ID --repo SivanCola/deepzora --exit-status
```

GitHub Release 出现或 Pages 钩子返回成功，都不能替代整条链路完成。确认上述阶段
全部成功，并查看以下公开页面：

- [中文官网下载区](https://deepzora.app/#download)与[英文官网下载区](https://deepzora.app/en/#download)：均显示目标版本。
- [官网版本记录](https://deepzora.app/release.json)：`version`、`sourceSHA`、`commitSHA`、`builtAt` 和文件清单可追溯。
- 下载链接指向 `https://downloads.deepzora.app/desktop/desktop-v0.0.9/…`；选择正确的系统和架构，抽查真实下载。

可在终端核对公开元数据。`sourceSHA` 是产品发行源码，`commitSHA` 是构建官网的
`main` 源码，两者职责不同，不要求相等：

```sh
curl --fail --silent --show-error 'https://deepzora.app/release.json' \
  | jq '{version, sourceSHA, commitSHA, builtAt, files: (.files | length)}'
curl --fail --silent --show-error 'https://downloads.deepzora.app/website/latest.json' | jq .
```

官网生产构建读取 R2 的已验证版本指针，再校验不可变 `mirror.json` 的摘要、大小、
源码和包矩阵，生成中英页面。无需每版手改 `website/src/release.json`，该文件用于
离线开发和 PR 预览。官网部署验收最多等待 15 分钟，超过时按下节恢复。

Windows 从 v0.0.8 起只展示安装包。候选验收还要求从真实 v0.0.7 安装版、便携版
分别升级、重启并保留数据；发布后的公开通道检查是额外门禁。旧清单必需的两个
`*-Portable.zip` 条目只保留已签名的迁移说明 ZIP，不含程序，不能删除条目或改指向
EXE。详情见[候选机制说明](RELEASE_CANDIDATES.zh-CN.md)。恢复被新版替代的历史
版本时，公开更新检查可按策略跳过，不会倒退更新指针。

## 六、失败后的定位与恢复

### 1. 先确定失败阶段

保留失败运行链接，在 Actions 查看红色 job/step。CLI 可读取日志：

```sh
gh run view RUN_ID --repo SivanCola/deepzora --log-failed
```

若仍在执行，先等它完成或确认不再需要后取消对应运行，避免同一候选存在多个发布。
修复凭据、权限或外部服务问题后再重试；不放宽主分支、标签、签名或审批保护。

| 失败位置或症状 | 处理方式 |
| --- | --- |
| 发布说明验证、署名解析失败 | 修复目录结构、中英内容、编号或 GitHub 权限，审核并合并后重新准备候选 |
| 签名预检、签名、公证失败 | 按“需要人工维护的情况”排查；外部配置修复后可重试同一未发布候选，源码变化则准备新候选，不能批准失败候选 |
| 构建、源码 CI、安装或原生验收失败 | 根据日志修复；代码变更合入后准备并验收新候选 |
| 准备候选时提示正式标签已存在 | 先检查同版本是否已公开或部分发布；已有发布进入恢复流程，不移动或删除标签 |
| 尚未发布的 payload 过期或候选被撤销 | 重新准备；被撤销候选不能通过恢复绕过限制 |
| 审批前的标签身份/规则预检失败 | 核对 `RELEASE_TAG_TOKEN`、`RELEASE_TAG_ACTOR` 与实时标签规则，修复授权后恢复或重新发起尚未激活的发布 |
| 正式标签、CLI、npm、Desktop、Homebrew、公开 postflight 中断 | 按“正式发布恢复”，复用同一候选 |
| 产品公开验收已通过，仅 R2 或官网失败 | 按“仅恢复 R2/官网”，无需重发 npm 和 GitHub Releases |
| 最后的旧版 Windows 公开更新验收失败 | 保留公开版本及文件，检查报告、通道和网络；瞬时故障可重跑失败 job，产品问题修复后发新补丁版本，不用重签覆盖已发布安装包 |

**仅外部配置或服务故障，且候选尚未公开时：**确认源码、控制工作流和发布说明
仍是计划发布的输入，所需 artifact 尚未过期，然后重跑原运行的失败任务：

```sh
gh run rerun RUN_ID --repo SivanCola/deepzora --failed
gh run watch RUN_ID --repo SivanCola/deepzora --exit-status
```

例如 Apple 协议接受后可重跑失败的 macOS 任务，复用已通过的 Windows 签名产物。
重试后仍须全部验收、封存成功，并核对最终字节及来源。若 `main` 已合入本版应包含
的新改动，不能用旧运行重试代替从新源码准备候选。正式版已公开时使用下文恢复流程。

### 2. 正式发布恢复：使用同一候选

先确认没有同一候选仍在运行，保留原候选 ID。以下命令派发 `recover`，随后仍需
维护者审批，并等待产品验收与 R2/官网任务完成：

```sh
candidate_id='粘贴原来的完整候选ID'
./scripts/release-stable.sh "$candidate_id" recover
./scripts/release-approve.sh "$candidate_id"
```

恢复复用原封存文件，核对已存在的标签和发布渠道，不重新构建或签名，也不把标签
移到新的 `main`。如果产品已部分公开而原候选文件不可用，停止并核对封存证据和
已公开字节，不直接用新构建覆盖相同版本；必要时修复后发新的版本号。

### 3. 仅恢复 R2 同步或官网部署

前提：目标版本的产品公开渠道验收成功，CLI 与 Desktop 正式标签指向同一源码。
执行下面命令即可重新同步、校验、触发官网并验收：

```sh
release_version=0.0.9
gh workflow run release-mirror.yml --repo SivanCola/deepzora --ref main \
  -f version="$release_version"
gh run list --repo SivanCola/deepzora --workflow release-mirror.yml --branch main --limit 10
gh run watch RUN_ID --repo SivanCola/deepzora --exit-status
```

如果只需网页操作，在
[Sync release downloads to R2](https://github.com/SivanCola/deepzora/actions/workflows/release-mirror.yml)
选择 **Run workflow → main**，填目标 `version`，再确认运行成功。

| 具体问题 | 诊断与重试 |
| --- | --- |
| R2 上传/下载超时、临时网络故障 | 修复连接后重跑镜像；已存在且一致的对象会被复用并重新回读 |
| `will not overwrite`、摘要或大小不匹配 | 停止，核对官方 Release、R2 对象及报告；不能靠强制覆盖通过校验 |
| 指针条件写入冲突或 `Website pointer changed after mirror` | 检查并发/较新版本，确认状态后重跑；旧版回补不会降级新版 |
| 缺少钩子、钩子 HTTP 错误 | 核对 Pages 的 main 部署钩子和 GitHub 加密 Secret；修复后重跑镜像 |
| Pages 构建失败、官网验收超时或页面仍旧 | 到 `deepzora-website` 查看对应 main 构建日志，核对远端清单及部署；修复后重跑镜像 |

失败不会把部分包切入官网；构建失败保留上一次成功部署。若超时后 Pages 才成功，
先核对生产记录，再重跑镜像取得本次完整验收成功的记录。恢复旧版时，较新的版本
指针会保留，官网部署步骤可能明确跳过，不能据此要求官网显示旧版。

### 4. 已发布版本本身有缺陷

修复产品并发布新的补丁版本。`recover` 用于完成原候选的中断发布，不会把修复代码
装进旧候选。官网代码回退可按 [Pages 回滚说明](../website/README.md)操作，但
不等于降级产品、撤回 npm 包或回退下载指针；当前镜像不提供产品版本降级命令。

## 七、可选：只演练、不公开发布

在需要检验签名和原生验收链路时使用 `rehearsal=true`，可指定已有审核记录的版本。
这仍需要有效的对应签名配置和源码 CI，不能作为绕过正式校验的捷径：

```sh
gh workflow run release-candidate.yml --repo SivanCola/deepzora --ref main \
  -f version=0.0.8 -f rehearsal=true
```

成功封存后复制 rehearsal 候选 ID，独立复核跨运行复用的来源和文件证据：

```sh
candidate_id='粘贴演练候选的完整ID'
gh workflow run release-candidate-verify.yml --repo SivanCola/deepzora --ref main \
  -f candidate_id="$candidate_id"
```

演练不会创建正式标签、公开 Release、npm 包或更新 Homebrew、R2、官网；演练候选
也不能转成正式发布候选。

## 八、每次发版留档

- [ ] 版本号、发布说明 PR 和计划发布的源码 SHA 已确认。
- [ ] 候选运行成功，ID、Product / Control SHA、安装验收结果已记录。
- [ ] 本候选的 Windows Certum 签名／时间戳和 Apple 公证／票据校验成功；v0.0.8 起的 Windows 两种旧安装模式升级证据通过。
- [ ] 正式发布经过维护者审批，运行链接已记录。
- [ ] 产品公开 postflight、R2 校验、官网部署验收全部成功。
- [ ] 当前版本要求的旧版 Windows 公开更新验收在 x64、ARM64 上通过。
- [ ] 中英文页面、版本记录和实际下载已确认。
- [ ] 保存发布 ledger、候选证据及 R2 校验报告的运行/artifact 链接；相关证据通常保留 90 天。

已完成的无人值守端到端基线：正式版 `0.0.8`，源码
`ee8163ad5a7d281931f0ba76c440176908d3feec`，候选
`v0.0.8-ee8163ad5a7d-e2df9aac4402`。
[候选运行](https://github.com/SivanCola/deepzora/actions/runs/43421221241)
在 Apple 账户接受新版协议后第二次尝试成功，保留第一次已验收的 Windows 签名文件；
[正式发布运行](https://github.com/SivanCola/deepzora/actions/runs/43802907302)
于 2026-11-16 全部成功。两条运行共同覆盖 Certum 签名、三个 macOS 目标公证、旧版
升级验收、CLI/npm/Homebrew、33 个 R2 对象回读及双语官网。发布后的两份
`published-windows-update-0.0.8-<arch>-1` 报告确认旧更新器下载的安装包与封存哈希一致，
且 Minisign、Authenticode 和时间戳有效。这是历史验收基线，不替代下一版的验收。

相关入口：[候选机制说明](RELEASE_CANDIDATES.zh-CN.md)、
[R2 维护说明](R2_DOWNLOAD_MIRROR.md)、[官网配置](../website/README.md)、
[代码签名政策](../CODE_SIGNING_POLICY.zh-CN.md)。
