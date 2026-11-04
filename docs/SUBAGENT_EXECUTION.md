# Subagent execution / 子代理执行

## Contract / 契约

`spawn_agents` accepts 1–8 independent tasks and waits for the batch. Each item
requires `title` and `task`; optional `profile`, `read_only` and `tools` narrow
the worker's configuration. A present `tools` list must be nonempty. Invalid
batch inputs, profiles, tool sets or model references start no child provider.
Once execution begins, a failed child does not cancel independent siblings.

`spawn_agents` 每次接收 1–8 个独立任务并等待整批结束。每项必须包含 `title` 和
`task`，可选 `profile`、`read_only`、`tools` 用于选择档案或缩小能力范围。显式
`tools` 列表不能为空。批次预检失败不会启动子模型；执行开始后，单项失败不取消其他任务。

```json
{
  "tasks": [
    {"title": "Review login", "task": "Review login and logout; cite file locations.", "profile": "review", "read_only": true},
    {"title": "Update help", "task": "Update the CLI help examples for the new syntax."}
  ]
}
```

Every child starts a fresh session with its task, profile prompt, workspace
facts and explicitly selected attachments. It does not inherit the parent's
conversation. Children cannot delegate, including through skill or capability
proxies. Plan mode and read-only profiles cannot be widened by call arguments.

每个子代理从新会话开始，只接收任务、档案提示词、工作区事实及明确传入的附件。
子代理不继承父对话，也不能通过 skill 或 capability 代理继续委派。调用参数不能
解除计划模式或只读档案的限制。

## Scheduling / 调度

The session shares one bounded execution pool and one writer slot across
ordinary delegation and professional skills. Read-only tasks may run together
and alongside one writer. Writable tasks run serially in batch order, including
their shell and MCP work. Waiting writers consume no execution slot. Configured mutation-capable tool hooks also require the writer slot, even for a read-only profile.
`max_subagent_concurrency` defaults to 6 and is bounded to 1–8. Retired depth
and parallel-writer settings no longer control execution.

普通委派与专业 skill 共用会话级并发池和一个写任务槽。只读任务可并行，也可与一个
写任务同时运行；可写任务按批次顺序串行，包括其 shell 和 MCP 操作。等待写槽的任务
不占执行槽。只读档案若启用可能写入的工具 hooks，也使用写任务槽。`max_subagent_concurrency` 默认 6、范围 1–8；旧嵌套深度和并行写数量
设置不再控制执行。

Cancellation waits for actual execution and cleanup before releasing the
writer slot. Workspace leases, permission rules, source-version observations
and file mutation locks still apply. Parallel readers are not a workspace
snapshot, and the scheduler is not a new cross-process transaction system.

取消后等待执行实际退出及清理完成，再释放写槽。工作区租约、权限规则、文件来源版本
检查和修改锁继续生效。并行读取不构成工作区一致性快照，调度器也不提供新的跨进程事务。

## Results and compatibility / 结果与兼容

Children finish with prose. Results contain execution `status`, a bounded
`head`, `more`, an archive `ref` when persisted, observed `read`/`wrote` paths,
and applicable error/retryability information. `paths_more` means the path list
was shortened to keep the result bounded. These are observations, not quality
certification or an exhaustive inventory of shell/MCP side effects.

子代理用文字回答结束。结果包含执行状态、有界预览、剩余内容标记、可用时的归档引用、
观察到的读写路径及错误／可重试信息。`paths_more` 表示路径列表因输出预算而缩短。
这些是执行事实，不是质量认证，也不保证列出 shell／MCP 的全部副作用。

`read_subagent_result` retains UTF-8 byte pagination and conversation/workspace
ownership checks, including retained partial failures. Historical task/fleet
records remain readable. New work cannot use `task`, `read_only_task`,
`parallel_tasks`, `fleet`, `complete_subtask`, continuation, background delegation,
dependency graphs or nested delegation. Retrying means starting a fresh task.
Ordinary background shell commands and workspace features remain independent.

`read_subagent_result` 保留 UTF-8 字节分页、会话／工作区归属检查及失败后部分结果读取。
旧 task/fleet 历史仍可查看；新任务不再使用旧工具家族、结构化完成报告、续跑、后台委派、
依赖图或嵌套委派。重试需要新建任务。普通后台 shell 命令与工作区功能独立保留。

Profile editing, model/effort defaults, tool restrictions and desktop try runs
remain supported. Try runs are read-only and temporary. Updated schemas and
guidance change the provider prefix once on upgrade; historical provider-visible
messages are not rewritten. Goal and ordinary-turn completion retain Deepzora's
existing model-owned completion semantics.

继续支持档案编辑、模型／推理默认设置、工具限制和桌面试运行。试运行只读且临时。
工具 schema 和指导文字升级会使缓存前缀变化一次，不改写模型可见历史；Goal 和普通回合
继续采用 Deepzora 现有的模型判断完成语义。
