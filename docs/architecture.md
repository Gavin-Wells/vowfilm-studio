# 系统功能与目录架构

本文描述当前代码。系统由 React 页面、同源 API 代理、单实例 Go 应用、SQL 元数据和本地媒体目录组成。组织订阅、分布式队列与对象存储属于后续设计。

## 功能与入口

| 用户任务                     | 页面 / 模式                                                            | 服务端主要实现                                                    |
| ---------------------------- | ---------------------------------------------------------------------- | ----------------------------------------------------------------- |
| 选择模板、填写故事或商品资料 | `app/new/page.tsx`                                                     | `http.go`、`creative_defaults.go`、`template_plan.go`             |
| 固定婚庆模板                 | `/`，`creationMode=template`                                           | `templates/catalog.json`、`template_plan.go`、`template_music.go` |
| 婚礼故事成片                 | `/wedding`，`creationMode=template`，`templateId=wedding-story-guided` | `wedding.go`、`wedding_automatic.go`、`wedding_render.go`         |
| 自由创作与局部重做           | `/`，`creationMode=agent`                                              | `treatment.go`、`workflow.go`、`provider.go`、`render.go`         |
| 电商短片                     | `/`，`creationMode=commerce-ref` 或 `scene=commerce`                   | `advertising.go`、`commerce_pack.go`、`video_model.go`            |
| 账户、账单、管理与配置       | `/account`、`/billing`、`/admin`、`/settings`                          | `auth.go`、`platform.go`、`settings.go`、`internal/platform`      |

婚礼故事在共享目录中声明 `engine=wedding-guided`，没有增加与 `template` 平行的 creationMode。前端展示与服务端校验读取同一份 `catalog.json`，历史固定模板来自 `archive.json`。

## 目录职责

```text
app/
  page.tsx                     通用工程工作台
  new/page.tsx                 模板、Agent、带货快创入口
  wedding/page.tsx             一键婚礼成片与 14 步材料面板
  account|billing|admin|.../    账号、计费、管理与配置
  api/[...path]/route.ts       网页到 Go 的同源代理
components/
  account-provider.tsx        会话与页面访问控制
  studio-nav-links.tsx        账户、余额、管理快捷入口
  creative-fields.tsx         按场景切换创作表单
  template-card.tsx           模板预览与选择
  task-quote.tsx              生成前报价确认
  ui/                        通用 UI 组件
lib/
  api.ts                     客户端请求与错误
  types.ts                   工程与婚礼流程类型
  creative.ts                场景默认值与草稿
  commerce.ts                电商模式与时长选项
  templates.ts               共享模板目录适配
server/
  cmd/server/                HTTP 服务入口
  cmd/migrate-db/             数据库迁移命令
  cmd/export-project/         工程导出命令
  internal/config/           环境配置
  internal/domain/           Project、Shot、WeddingWorkflow 等数据类型
  internal/platform/         账户、权限、报价、账本服务及仓库接口
  internal/storage/          SQL 实现、事务与编号迁移
  internal/templates/        当前和历史模板目录
  internal/advertising/      原版广告 Prompt、适配契约与来源
  internal/wedding/          婚礼阶段规则、原版 Prompt、来源及打包
  internal/studio/           HTTP、生成编排、供应商适配与媒体处理
  requirements-wedding.txt   真实旁白对齐所需 Python 依赖
public/
  templates/                 原版故事采集卡与许可证
  demo/                      演示封面、字幕与单独分发的视频
scripts/                     开发启动与演示导出
docs/                        当前使用说明、架构及设计记录
design-system/               早期视觉参考
data/                        SQL、配置、项目媒体（不提交）
deliverables/                成片、素材和检查报告（不提交）
```

`studio` 是应用编排层，按功能文件划分。通用流程留在 `workflow.go`；婚礼流程、旁白对齐和渲染使用 `wedding_*`；电商时长、打包和身份参考使用 `commerce_*`。原始 Prompt 与纯阶段规则独立于 HTTP 和数据库，避免修改业务适配时改变上游原文。

## 三条生成链路

### 婚礼故事

用户填写故事 → 选择一键生成 → 确认报价 → 原版规则生成文案 → 生成旁白 → Whisper 对齐真实音频 → 按语义时间点生成分镜 → 首帧与统一人物参考 → 逐镜视频 → 全长音乐 → 混音 → 字幕 → MP4/SRT/剪辑表。

`wedding_automatic.go` 保存已提交任务和已下载文件；`wedding_align.py` 只处理真实音频及原文对齐；`wedding_render.go` 消费确定的时间轴和音轨。依赖检查在一键入口执行。默认 90 秒、16:9；没有照片时生成虚构形象，有照片时将用户提供的阶段照片作为参考。

同一工程保留 14 步逐步制作模式，支持材料回传、SHA-256 校验、当前版本确认与返工。自动完成使用 `automated`，人工确认保存身份、反馈和版本，两者不混用。原始文件置于 `internal/wedding/upstream`，全量摘要测试防止改写 Prompt。

### 电商

商品事实、口播和可选参考图 → v3 广告分镜 → 校验总时长 → 按模型上限合并生成组 → 首组确立人物参考 → 其余组并发生成 → 拼接并保留原生音轨 → 成片。

工程支持 10–60 秒；当前代码对 H3 使用 15 秒单次上限，对 Seedance 使用 30 秒，其余模型按 15 秒处理。这是本系统的适配配置，新模型需要在 `video_model.go` 中核对后维护。

`Shot.Duration` 表示分镜时间；`GenerateGroup` 标识生成组，`GenerateUnit` 表示组首镜，`GenerateSeconds` 是该组请求时长。不要把细分镜数当作上游任务数。分组、参考和重试的实现分别在 `commerce_pack.go`、`template_plan.go` 与 `workflow.go`。

### 通用 Agent 与固定模板

Agent 先生成导演方案与章节，再分批规划镜头。固定婚庆模板从版本化目录直接生成镜头计划。两者共用参考素材绑定、视频任务、章节配乐、FFmpeg 合成和局部重做；固定模板与婚礼故事模板在 `workflow.go` 分流。

## 数据、费用与恢复

- 项目快照及婚礼阶段数据进入 SQL `projects`，媒体保存在项目目录。阶段 JSON 通过现有快照持久化，不另建一套 Python 工作流数据库。
- 所有前端通过同源代理访问 Go；Go 执行会话、权限和项目归属检查。模型密钥只在服务端，`provider.json` 不进入 Git。
- 生成前保存报价、项目指纹和价格版本，执行时预留额度，成功结算、失败释放。婚礼自动流程使用一次 `generate` 报价；分阶段模式按对应动作报价。
- 项目未编排时按当前规则估算镜头/生成组数量，已编排电商按生成组报价；该报价不等于供应商实际成本明细。
- 任务 ID、尝试次数与已完成文件用于恢复。进程重启保留可恢复记录，但仍需要用户继续任务；不能把单实例应用视作持久队列或多 worker 系统。

## 开发验证与实际边界

从仓库根目录运行 `npx tsc --noEmit`、`npm run lint`、`npm run build`，后端运行 `go -C server test -race ./...`。原版文件摘要、阶段门禁、过期确认、真实媒体探测、首帧请求、任务复用、电商分组和身份参考都有对应测试。常规测试不调用付费供应商，live 测试需要显式开启。

已交付的 90 秒婚礼演示证明真实媒体生成与完整渲染可用。尚未完成一次仅输入故事、从零运行全部付费服务的婚礼一键实测；技术检查也不等于自动验收脸部、手部、事实或口播质量。详细材料与验证方式见各功能文档。
