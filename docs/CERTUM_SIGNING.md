# Windows cloud signing / Windows 云签名

Windows releases use Certum SimplySign. The repository needs `CERTUM_USERNAME`,
`CERTUM_OTP_URI` (the complete TOTP provisioning URI), and `CERTUM_KEY_ID` (the
40-character SHA-1 certificate thumbprint). Never commit provisioning data.
The certificate private key remains in Certum's cloud. CI holds the credentials
needed to authenticate signing, so access to these secrets is signing authority.

Windows 发布使用 Certum SimplySign。配置以上三个 Secret；不要提交二维码或 TOTP
配置。证书私钥保留在云端，但 CI 凭据能授权签名，应按签名权限保护。

## Unattended GitHub Actions signing / GitHub Actions 无人值守签名

The existing release workflow performs login, payload signing, installer signing
and verification on GitHub-hosted Windows runners. No local Windows computer or
per-release phone code is needed once all three credentials are configured.
Keep the `release` environment approval gate: approving a release does not
require manually authenticating each signing operation.

现有发布工作流在 GitHub 托管的 Windows runner 上完成登录、程序签名、安装包签名和
验证。配置齐三个凭据后，不需要本地 Windows 电脑或逐次输入手机验证码。保留
`release` 环境的发布审批；发布审批与签名过程中的人工登录是两回事。

Store the complete original provisioning URI as `CERTUM_OTP_URI` in Actions
secrets, never in a workflow input, repository file or chat. A six-digit phone
code is not a replacement. If the original URI/QR backup is unavailable, the
current automation cannot generate login tokens. Obtain an authorized new
provisioning configuration from Certum before enabling live signing; do not
reset an existing phone enrollment without its owner's approval. Switching to
GitHub Actions cannot recover a missing TOTP secret.

将原始完整配置 URI 保存为 Actions Secret `CERTUM_OTP_URI`，不要放入工作流输入、
仓库文件或聊天。手机六位验证码不能替代它。如果没有原始 URI／二维码备份，当前
自动化无法生成登录验证码，必须先通过 Certum 获取获授权的新配置，才能启用实际
签名。未经所有者批准，不得重置已有手机绑定；改用 GitHub Actions 无法恢复缺失的
TOTP 密钥。

The smoke workflow checks credential format before compiling probes. The shared
login action checks it again, pins the SimplySign installer SHA-256 as well as
its publisher signature, and keeps authentication screenshots disabled. The
owned login adapter waits for two editable controls and an OK button, rather
than treating the first visible splash window as the login form. It ignores
background windows when identifying acknowledgement dialogs. A credential-free
probe of the official client runs only on disposable GitHub-hosted Windows
runners; deterministic tests reproduce the splash/disappearance/form ordering.
Both signing workflows disconnect the cloud session even when signing fails. Format
validation does not prove that the account, certificate or TOTP seed is valid;
only a successful live signing test does.

签名测试在编译探针前检查凭据格式；共享登录 Action 再次检查，并同时验证 SimplySign
安装器的固定 SHA-256 和发布者签名，保持登录截图关闭。仓库维护的登录适配器等待两个
可编辑控件和 OK 按钮出现，不把最先出现的启动窗口当作登录表单，识别确认弹窗时也会
排除后台窗口。官方客户端的无凭据探测只在可丢弃的 GitHub 托管 Windows runner 上
运行；确定性测试覆盖“启动窗出现、消失、登录表单出现”的顺序。两个签名工作流在
签名失败时也会断开云签名会话。格式检查不能证明账号、证书或 TOTP 密钥有效，仍须实际签名成功。

Historical Stable versions `0.0.1`–`0.0.6` retain their explicit unsigned policy.
Unlisted versions such as `0.0.7` require Certum automatically and cannot fall
back to unsigned output when credentials or signing fail. Prepare and approve
a new signed candidate rather than silently replacing an existing release.

历史稳定版 `0.0.1`～`0.0.6` 保留明确批准的未签名策略。`0.0.7` 等未列入例外的版本
自动要求 Certum 签名，凭据缺失或签名失败均不得降级为未签名发布。应准备并审批新的
已签名候选，不应直接替换既有发布包。

## Live qualification / 实际签名验收

Run **Certum signing smoke test** on protected `main`, approve the `release`
environment, and require successful login, signing, trust, signer and timestamp
verification. This test publishes nothing. The authentication implementation is
vendored from an audited upstream commit with its MIT notice; the local window
readiness adapter is covered by regression tests. Authentication screenshots
are disabled. Review upstream changes before updating the vendored code.

先在 `main` 手动运行签名测试并批准 `release` 环境。测试不发布产物，必须通过
登录、实际签名、信任链、签名者及时间戳检查。认证实现从已审查的上游固定提交引入
仓库并保留 MIT 声明，本地窗口就绪适配器有回归测试。更新上游代码前需要审查，登录
截图保持关闭。

Release builds retain native x64 and ARM64 startup tests. Their exact payloads
and generated NSIS identity files pass through same-run artifacts to serialized
x64 signing jobs. Those jobs sign the PE manifest, bind the minisign payload
manifest, rebuild packages, sign installers, verify the version-appropriate ZIP and expected
signer/timestamps, and only then produce final minisign-signed release bundles.
Any signing failure blocks publication and signing attestation.

正式发布保留 x64/ARM64 原生构建与启动测试，再将同一运行中的产物交给 x64 签名任务。
先签内部 PE 文件，再绑定清单、重打包、签安装器并验证对应版本的 ZIP，最后生成发布包。
从 v0.0.8 起 ZIP 仅包含旧更新清单所需的迁移说明，不含程序；仍使用 Minisign 签名。
任何签名失败都会阻止发布和签名证明。

The current signing contract lives in `.github/release-signing.yml` and is verified by `cmd/release-signing-contract`. `RELEASE_SIGNING_ATTESTATION` records an explicitly verified current fingerprint for standalone recovery. Candidate publications use their sealed evidence. No SignPath request or historical receipt format is supported.

当前签名契约位于 `.github/release-signing.yml`，由 `cmd/release-signing-contract` 验证。独立恢复使用 `RELEASE_SIGNING_ATTESTATION` 登记已验证的当前指纹；候选发布使用封存证明。不支持 SignPath 请求及历史回执格式。

A successful standalone preflight uploads `verified-signing-contract-*` evidence
and prints the maintainer command for promoting its fingerprint to the recovery
variable in the job summary. `GITHUB_TOKEN` cannot modify repository variables;
the workflow does not need a privileged personal token. Promote only the exact
fingerprint from a run whose two Windows signing jobs succeeded. Official
orchestrated releases use successful same-run preflight evidence instead.

独立预检成功后上传 `verified-signing-contract-*` 证明，并在任务摘要中给出由维护者
将指纹写入恢复变量的命令。`GITHUB_TOKEN` 无权修改仓库变量，无需为此添加高权限
个人令牌。仅登记两个 Windows 签名任务均成功的运行所验证的准确指纹；正式编排发布
使用同一运行内的预检成功结果。
