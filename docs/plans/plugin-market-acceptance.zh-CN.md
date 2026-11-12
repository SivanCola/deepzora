# 插件市场交付与验收（2026-11-12）

**交付范围。** [社区仓库](https://github.com/SivanCola/deepzora-plugins) 管理投稿、CI、版本保留及发布。Deepzora 负责解析、安装、状态恢复、CLI 和桌面设置。面向作者的入口是元数据 PR；社区托管的包也可把源码放在同一 PR。不要把投稿 CI 通过解释为已经审核。

| 设计步骤 | 验收证据 |
|---|---|
| 真实投稿 | [Review Checklist PR #1](https://github.com/SivanCola/deepzora-plugins/pull/1) 提交 v2 资源包。配置 main 分支要求 validate 和一份批准，独立审核待完成。 |
| 实际分发 | [初始 dist 发布](https://github.com/SivanCola/deepzora-plugins/actions/runs/43201978028) 和[独立 topic 索引任务](https://github.com/SivanCola/deepzora-plugins/actions/runs/43203590124) 成功；正式目录为空符合当前发布状态。 |
| 候选包安装 | 下载[投稿 CI](https://github.com/SivanCola/deepzora-plugins/actions/runs/43202144947) 的 catalog artifact，作为自定义目录输入真实 CLI。搜索、预览、回传 planId、从 GitHub 按提交获取并安装、doctor 成功。在临时 Deepzora home 的安装记录中读到 `762d58f80167bdc0059eacb230afb88c32e42df3`。这不是绕过审核的线上社区安装。 |
| 可重复端到端验收 | 构建 CLI 后执行 `python3 scripts/plugin-market-e2e.py /absolute/path/to/deepzora`。临时 Git 上游、两代提交、移动 tag、错误 planId、撤回、禁用升级、不同来源同名、Claude 兼容包均有断言；脚本不会写真实用户配置。CI 已接入 Linux、macOS 和 Windows，远端状态应以对应 PR 检查为准。 |
| 中断恢复 | Go 测试模拟目录换好但状态尚未提交、状态已提交两个崩溃边界，验证恢复结果及二次恢复的幂等性。 |
| 桌面 | Go 后端插件测试、前端构建、设置交互测试及 Playwright 中英文设置页矩阵。浏览器使用测试桥，验证发现 → 预览 → 安装时计划 ID 的传递；真实下载由上面的 CLI 验收覆盖。 |

**对原设计的落地调整。** 生产索引使用扁平的 `releases[]`，一个元素就是一个可独立撤回的版本；窗口按 ID 与来源聚合，不要求索引中的嵌套层级与 UI 一致。社区使用 dist 分支发布静态索引，安装包内容从固定 Git SHA 获取。未引入 npm、Bun、无提示自动升级或单独的二进制镜像。可选最低版本与平台属于发布元数据，真实可运行性仍由安装计划、doctor 和宿主激活结果判断。

**上线顺序。** 合并通过审核的插件投稿后，publish 工作流会把第一条已审核记录写入 dist；合并并发布 Deepzora 客户端后，现有用户才会看到新入口。两个发布独立，均无需更改已提交的插件源码。
