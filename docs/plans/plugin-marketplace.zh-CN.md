# Deepzora 插件提交、发现与在线安装方案

状态：原始设计与取舍，以下示例为设计契约，不是已发布版本的用户手册。实现基于 `943bdd50a`；实际命令见[插件包文档](../PLUGIN_PACKAGES.zh-CN.md)，交付差异和实测证据见[验收记录](plugin-market-acceptance.zh-CN.md)。

## 决策

第一版建立一个由 GitHub PR 维护的**版本目录**。作者继续在自己的仓库发布，社区审核一个确定的 Git 提交。桌面端和 CLI 使用同一个 Go 客户端查目录，选中版本后仍交给原有安装器。作者自己发布即可通过 URL 安装；审核只决定哪个版本进入社区推荐。第二阶段再用 GitHub topic 做未审核发现。首版不要求 npm 包、Bun、插件托管服务或作者登录 Deepzora。

## 从现有代码出发

| 现有能力 | 设计如何使用 |
|---|---|
| [pluginpkg](../../internal/pluginpkg/pluginpkg.go) 解析原生、Codex、Claude 清单；[原生 v2](../../internal/pluginpkg/manifest_v2.go) 声明资源、runtime 和能力契约 | 继续以真实包为准。索引提供摘要，不能给包增加能力。兼容包显示实际映射及跳过项。 |
| [installsource](../../internal/installsource/plugin_package.go) 支持本地及 GitHub 仓库/子目录，并能解释部分 Claude marketplace；GitHub 计划检查源码树、记录 commit；复制安装先暂存、检查能力，再替换目录 | 目录条目指向一个具体包子目录，只产生一个安装动作。保留原有按 URL、本地路径、开发链接和 marketplace 导入入口。 |
| [安装计划](../../internal/installsource/install_source.go) 支持 planId，Hook、MCP、runtime 在计划中有不同风险说明 | 扩展计划以绑定目录发布身份；在线确认必须回传计划 ID。 |
| [桌面后端](../../desktop/plugin_packages_app.go) 已有预览、安装、诊断、启停和更新；[插件设置页](../../desktop/frontend/src/components/CapabilitiesPanel.tsx) 已经展示安装计划 | 在现有设置页增加“发现”。现有安装选项未透传 planId；现有 UpdatePlugin 会按旧 source 直接 replace。社区更新需改为预览新发布再确认。 |
| [CLI](../../internal/cli/plugin.go) 已有 install/list/show/doctor 等；全局状态在 Deepzora home 的 plugin-packages.json，安装目录以本地 name 命名 | 不改变旧命令语义；给安装记录添加可选来源字段，旧记录仍可读。 |
| [MCP Registry 客户端](../../internal/mcpregistry/registry.go) 只在用户浏览/安装时访问网络，支持缓存回退 | 沿用这个产品约定；插件市场不进入会话启动、模型请求或工具发现路径。 |

## 一、提交和发布

1. **作者准备包。** 提供仓库、许可证、README、版本、包所在子目录。用现有 `deepzora plugin install ./path --dry-run` 本地检查。在提交流程中首选原生 v2，Codex/Claude 包有可映射能力也可收录。runtime 若依赖外部命令，README 列出平台和前置条件；首版不替作者执行构建或依赖安装脚本。
2. **作者向独立目录仓库提元数据 PR。** 维护者可先创建如 `SivanCola/deepzora-plugins` 的仓库（拟名）。一条发布提交稳定 ID、仓库、路径、完整 commit、版本、名称和中英文简介。Git tag 便于阅读，不作安装依据。仅修改插件仓库的 main 或 tag，不改变已收录版本。
3. **CI 生成差异供审核。** 受信任的校验器只读取指定提交，用与客户端同源的 Go 解析器生成清单类型、能力、runtime 命令、兼容性警告；校验版本、路径、重复 ID / 来源、重复版本换 SHA、许可证及平台声明。PR 中展示与上一发布相比新增的 hook 命令、MCP 启动目标、runtime、provides/requires 等。解析工作不启动插件、不加载 hook、不带发布凭据。对需要运行验证的包，另用隔离测试作业。
4. **维护者审核该版本。** 核对仓库归属与发布者、能力及其实际行为；有执行能力时检查相对上次审核的代码变化，不能只看能力数量。目录来源变更或维护权移交单独审核。审核人和 PR 记录由发布流程关联，不接受作者自填“已审核”。
5. **合并后发布静态索引。** 受保护分支的工作流重新校验，通过后发布版本化快照和稳定入口 `index-v1.json`。失败继续服务上一次成功快照。审核对象是“ID + 版本 + commit”，同一版本不允许重写；修复必须发新版本。撤回用 yanked 标记并附原因，停止推荐和新装，已安装用户收到通知，自行选择停用或卸载。

**索引最小契约（以下是示例数据）：**

```json
{
  "schemaVersion": 1,
  "revision": 42,
  "plugins": [{
    "id": "acme/review-kit",
    "name": "Review Kit",
    "summary": {"zh": "代码审阅命令和技能", "en": "Review commands and skills"},
    "source": {"repo": "acme/deepzora-tools", "path": "plugins/review-kit"},
    "releases": [{
      "version": "1.2.0",
      "commit": "0123456789abcdef0123456789abcdef01234567",
      "manifestName": "review-kit",
      "manifestKind": "deepzora",
      "minDeepzora": "0.0.5",
      "platforms": ["darwin", "linux", "windows"],
      "reviewPR": 123,
      "yanked": false
    }]
  }]
}
```

平台、最低宿主版本与能力摘要属于每个版本；平台声明与已测试平台分开展示。只在受信任的内置目录来源上赋予“社区已审核”标记。自定义 URL、第三方索引里的同名字段不能生成该标记。首版以 HTTPS 和受保护分支为分发信任根；将来若提供独立镜像，再增加签名快照和防降级验证。

## 二、发现、去重和已安装匹配

桌面“设置 → 插件”保留“已安装”“从 URL / 路径安装”，增加“发现”。默认展示社区审核版本，可按语言搜索名称、简介、作者、能力，按原生/兼容、可用于本机筛选。卡片展示来源、版本、主要能力和安装状态；详情链接到 README、准确的代码提交和审核 PR。目录下载失败时显示最近成功缓存及时间；没有缓存则提示重试，同时保留手动安装入口。建议 6 小时缓存、ETag 条件请求、手动刷新、限制响应大小。过期缓存可浏览，执行目录安装/更新前需在有效期内确认发布未撤回。

三个身份分别处理：

| 身份 | 用途 |
|---|---|
| 稳定目录 ID，例如 acme/review-kit | 搜索、订阅版本、关联历史；displayName 可改。 |
| 来源身份：GitHub 仓库标识 + 规范化子目录 | 合并社区目录与 topic 命中，也用于识别以前通过 URL 手动安装的包。仓库部分规范化，子目录保留大小写。 |
| 本地安装名，例如 review-kit | 沿用 plugins/<name> 和现有命令；用户已有别名继续保留。 |

同一来源和子目录只显示一张卡。社区审核到 1.2.0，上游另发了 1.3.0，主按钮仍安装 1.2.0；若展示新版，则单列“上游版本，未审核”。不同仓库即使同名也分别显示发布者。社区把源码复制到另一仓库时，只有经过审核的显式来源映射才可归为同一项目，原安装不自动切换渠道。相同本地名已被其他来源占用时展示冲突，允许选合法别名或取消，不隐式 replace。

旧状态优先按已有 source、子目录、commit 匹配；来源匹配但提交不匹配时标注“已从此仓库安装，非审核版本”。缺少可验证来源或使用本地 link 的安装保持“本地管理”，不能仅凭名称认领。

第二阶段：后台工作流定时扫描公开、未归档仓库的 `deepzora-plugin` topic；先支持仓库根目录的受支持清单，monorepo 通过显式提交通道指定子目录。生成与社区目录分开的候选索引，记录扫描 commit、时间；静态解析通过后才进入“第三方 · 未审核”。普通客户端不各自扫描 GitHub。第三方默认不进入社区推荐，不自动覆盖或更新。

## 三、在线安装和升级

1. **选择发布。** 新增 Go 包 `internal/plugincatalog`，集中实现搜索、缓存、去重和版本解析。Desktop/CLI 发送目录 ID、版本和本地目标，Go 从可信目录解析坐标；前端不能提交一组自称已审核的 URL 和 SHA。首版社区版本采用明确的 SemVer 规则，默认选最新兼容、非预发布、未撤回的版本。
2. **固定输入。** 给 installsource 增加明确的 repo/path/expectedCommit 输入，在解析 manifest **之前**获取并检出该 SHA，再校验包路径、名称和版本。不能只把 SHA 拼到 GitHub tree URL：当前准备流程对 tree ref 使用浅克隆的 `--branch`，并非通用的任意 SHA 安装接口。依赖无法满足时说明原因，不悄悄安装依赖插件。
3. **预览真实变更。** 从检出的包生成 installsource 计划：目标目录、实际版本、兼容跳过项、将启用的资源、可执行命令和完整信任提示。显示运行时前置依赖；区分“包可复制”和“本机可运行”。升级另显示旧→新 commit 及能力/命令差异。即使社区审核通过，也保留现有风险分级。
4. **确认同一计划。** Desktop 预览返回 planId，确认 API 必填；CLI 新增可回传的 `--plan-id`。目录 ID、版本、源路径、commit、目标别名、替换策略、旧安装身份纳入计划指纹；托管端在应用前复核计划、撤回状态及当前目标。目标或计划改变则返回“请重新预览”，不下载另一个版本作为替代。
5. **安装并登记。** 在线安装只走 copy；沿用暂存复制和复制后校验。插件状态增补可选 catalogId、channel、reviewedVersion 等字段，实际 source 和 commit 仍保留。升级保留本地名称与 enabled 状态（现有包安装路径会写 Enabled=true，需显式修正）。并发确认须对目标串行化，并在锁内复核前置状态。
6. **分别报告落盘和生效。** 沿用桌面扩展 generation / rebuild 及延迟生效路径。用户应能看到“已安装并生效”“已安装，等待会话允许重载”或“已安装，激活失败，查看诊断”。不要把 rebuild 失败显示成下载失败。目录替换与状态写入需一起恢复：现有复制函数在换目录成功后即删除备份，需延长备份到状态提交完成，保留恢复记录以处理进程中断。回到上一版只还原包和登记状态，不声称还原插件已产生的外部效果。

**首版更新策略：** 只提示，不静默执行。社区渠道比较同 ID 的审核发布；GitHub 手动安装、兼容 marketplace 和本地 link 仍归原来源管理。市场托管包从“已安装”点击更新也必须走目录固定版本计划；不可通过旧 UpdatePlugin 自动追踪上游 HEAD。用户可以固定版本或跳过本次更新。

**建议的 CLI（现有 URL 安装语法继续有效）：**

```sh
deepzora plugin search review
deepzora plugin info catalog:acme/review-kit
deepzora plugin install catalog:acme/review-kit@1.2.0 --dry-run
deepzora plugin install catalog:acme/review-kit@1.2.0 --yes --plan-id <预览返回的ID>
deepzora plugin updates
deepzora plugin update review-kit --dry-run
```

桌面新增 SearchPluginCatalog、PluginCatalogDetail 两个只读入口；安装、更新共用版本解析与计划确认服务，扩展现有 Go bridge 参数并从 Go 重新生成桌面契约。目录错误（离线/撤回/不兼容）、安装错误（计划变化/同名冲突/无法获取 SHA）与激活错误分开返回。

## 四、交付顺序与验收

| 阶段 | 交付内容 | 发布门槛 |
|---|---|---|
| 1. 社区渠道闭环 | 目录 schema、作者模板、静态校验/发布工作流、Go 客户端、固定 SHA 安装、CLI 搜索及预览确认 | 用含资源的原生包与有兼容跳过项的包，从 PR 到临时 Deepzora home 完整安装；移动 tag 后仍只装审核 SHA；预览变化、撤回或不同来源占名时不写盘。 |
| 2. 桌面及更新 | 发现/已安装联动、更新差异、保留禁用状态、失败恢复及诊断 | 用含 runtime 的测试包验证信任提示、取消零写入、禁用升级后仍禁用、重载延迟与激活失败不误报成功；模拟状态写入失败，旧版本仍可用。CLI 和 Desktop 解析同一版本。 |
| 3. 未审核发现 | topic 索引、独立来源筛选、显式来源映射 | 同 repo/path 多入口只展示一张卡；不同仓库同名不会覆盖；审核章不随 tag、fork 或候选索引迁移；离线启动会话不依赖市场。 |

跨平台验收至少覆盖 Windows 路径/文件占用、macOS 和 Linux 的 Git 安装与目录替换。保留旧状态文件、手动 URL、本地 link 的回归用例。后续若 Git 下载速度成为实际瓶颈，再设计可校验的发布压缩包和镜像；目录身份、版本审批及现有安装状态均可沿用。

参考：[Magpie 社区目录与投稿](https://github.com/magpie-community/plugins#the-market)、[GitHub topic 索引](https://github.com/magpie-community/plugins/blob/main/.github/workflows/tagged.yml)、[Deepzora 插件包说明](../PLUGIN_PACKAGES.zh-CN.md)、[Deepzora 运行时 v2](../EXTENSION_RUNTIME_V2.zh-CN.md)。
