# 团队实现

[English](TEAM.md)

Team 移植面向本机 macOS、Linux、Windows Desktop，进度见[交付清单](TEAM_PORT_STATUS.md)。
当前增量提供存储、编排和可选的 Controller/boot 席位适配，桌面宿主和界面尚未启用。

## 状态归属

`internal/teamroster` 管理带版本的名册、revision 校验、交换账本、群日志和轮次状态。
写入使用 Deepzora 的文件身份锁和严格原子替换。宿主驱动服务前必须取得写入租约；
Store 本身是持久化原语，不是授权边界。

`internal/team` 管理成员授权、有界交办、群发言顺序、计时和恢复，通过 `Panes` 接口
驱动会话。`Pane.Deliver` 只有在 inbox 持久化成功后才可返回成功；Controller 必须通过
`Settlement` 报告真实结束状态和实际消费的回复。

`internal/hostframe` 将不可信文本中的宿主标签转义，不授予执行权限。成员指令和消息
仍需遵守接收方的执行权限。

## 持久化与兼容

名册 schema 1 是新建的 Deepzora 数据，只接受 `read-only`、`workspace-write`、
`danger-full-access` 三种规范权限值；未指定权限由后续桌面宿主解析。不导入 Reasonix
数据。更高版本拒绝写入；损坏数据保留并报错。JSONL 末尾中断的半行在读取时跳过，
下次追加前清除。

同一请求 ID 只能重试完全相同的发送者、接收者、父交换、群组、回复目标、执行上限
和消息。重启先将未完成交换与轮次结算为中断，后续 Controller 适配必须拒绝已结算任务
在 inbox 中重放。

租约用于三平台上协作进程的互斥，不能隔离同一系统用户运行的其他程序。Windows
Shell 继续遵守 Deepzora 现有的当前系统账户执行约定。

## 验证

运行 `go test ./internal/teamroster ./internal/team ./internal/hostframe`，支持的平台补充
相同包的 `-race` 检查。覆盖 revision 冲突、损坏与未来版本、真实子进程租约竞争、
规范权限、幂等、交办循环、回复消费、群计时、审批契约及重启恢复。Windows 交叉编译
只证明可编译，原生运行仍是独立验收项。

普通 boot 保持既有模型请求与工具目录。团队 boot 注册一致的团队工具，将当前身份追加
到回合尾部。团队输入带有宿主来源及临时执行上限，不覆盖成员的长期权限；持久配置审批
提交时校验用户审阅的名册 revision。既有会话、预设、凭据和 Desktop RPC 格式不变。
提取代码的 MIT 许可证保留在 `internal/team/UPSTREAM_LICENSE`，同时覆盖名册与宿主
framing 包。
