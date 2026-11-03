# Deepzora changelog / 更新日志

## Unreleased — first-version baseline / 未发布：首版基线

- Consolidate delegation into `spawn_agents` (1–8 tasks), with parallel readers, serial writers and one shared runner for professional profiles. Retire background delegation, continuation, dependency graphs, nested delegation and structured subtask completion. Retained results remain readable.
- Remove unused readiness audit forwarding and plan acceptance/verification fields. Updated tool schemas change the cache prefix once on upgrade.
- 子代理统一为 `spawn_agents`（1–8 项），只读并行、写任务串行，专业档案共用同一执行入口。移除后台委派、续跑、依赖图、嵌套委派及结构化子任务完成报告，保留历史结果读取。
- 清理无生产端的 readiness 审计转发和计划验收／验证字段；工具 schema 升级使缓存前缀变化一次。详见 [子代理执行契约](docs/SUBAGENT_EXECUTION.md)。

- Current CLI, Electron desktop, Serve, ACP, bots, and SDK remain supported.
- Session storage, composer state, navigation, cancellation, and remote connections use current contracts only.
- Releases use sealed candidates, signed desktop artifacts, checksums, and provenance.
- Reasonix release history and obsolete migrations are excluded from this project's release catalog.

- 保留当前 CLI、Electron 桌面端、Serve、ACP、机器人及 SDK。
- 会话存储、输入状态、导航、取消操作和远程连接仅使用当前契约。
- 发布保留封存候选、桌面签名、校验和与来源证明。
- 本项目发布目录不继承 Reasonix 发布历史和旧迁移流程。

The first release has not been published. Reviewed notes live in [release-notes/releases.json](release-notes/releases.json).

首版尚未发布，发布准备记录见 [release-notes/releases.json](release-notes/releases.json)。
