# SaaS 平台化设计

> 后续规划文档。组织、订阅、队列、对象存储及本文件列出的阶段目标尚未全部实现；当前目录、功能和单实例边界见 [系统架构](architecture.md)。

本文把当前单用户、单 Go 实例的影片工作台扩展为可运营的多租户 SaaS。现有影片领域、积分冻结/结算规则和媒体处理流程继续复用；新增能力集中在平台层，避免把计费、租户和作业状态散落到 `studio` 包。

## 目标边界

平台的租户是 `organization`，用户通过 `membership` 加入一个或多个组织。项目、素材、报价、账单和作业都归属于组织；个人账户只是登录身份。所有读取和写入都必须带 `organization_id`，管理员权限也只在当前组织内生效。平台运营人员使用独立的 `platform_admin` 权限，不通过普通组织管理员绕过隔离。

第一阶段继续支持个人组织（注册时自动创建），因此现有用户不会被迫迁移。后续可邀请成员、转移项目和设置默认组织。删除组织采用软删除和保留期，媒体清理由异步任务执行。

## 核心数据模型

建议新增迁移 `002_saas.sql`：

| 表 | 关键字段 | 约束 |
| --- | --- | --- |
| organizations | id, name, slug, plan, status, created_at | slug 唯一；active/suspended/deleted |
| memberships | organization_id, user_id, role, status | `(organization_id,user_id)` 唯一；owner/admin/editor/viewer/billing |
| invitations | id, organization_id, email, role, token_hash, expires_at, accepted_at | token 只存摘要；同组织同邮箱最多一个未过期邀请 |
| subscriptions | organization_id, provider, external_id, status, period_start/end, cancel_at | 外部订阅号唯一；保存价格快照 |
| entitlements | organization_id, key, limit, used, period_start/end | 用于 seats、并行作业、存储、生成秒数和导出次数 |
| jobs | id, organization_id, project_id, kind, state, idempotency_key, attempts, available_at | 幂等键按组织唯一；状态变更写事件 |
| job_events | job_id, type, payload, created_at | 追加写，供恢复、审计和通知 |
| media_objects | id, organization_id, project_id, object_key, sha256, bytes, content_type, state | `(organization_id,sha256)` 可去重；pending/ready/deleted |
| api_keys | id, organization_id, name, hash, scopes, last_used_at, revoked_at | 只展示一次原文；按 hash 查找 |
| webhooks | id, organization_id, url, secret_hash, events, active | 发送记录和指数退避重试 |

现有 `users` 保留登录身份；`projects`、`quotes`、`charges`、`entries`、`audits` 增加 `organization_id` 并建立索引。迁移期间从用户当前归属创建个人组织，旧项目和账本全部归入该组织。所有 Repository 方法新增组织参数，禁止通过“先查项目再判断用户”的应用层补丁实现隔离。

## 订阅、额度与计费

积分账本仍是实际扣费账本；订阅只负责授予权益。每个动作执行前依次完成：权限检查 → 组织状态检查 → entitlement 预留 → 积分 quote/hold → 创建 job。任一步骤失败都回滚已完成的预留。任务完成、失败和取消分别结算或释放两套预留。

权益至少包括：最大成员数、并行作业数、月度生成秒数、存储字节数、单文件大小、保留天数和 API 请求速率。额度按 UTC 周期滚动，使用原子条件更新，不能依赖内存计数。超额返回稳定错误码 `entitlement_exceeded`，同时给出当前用量和下个重置时间。

支付供应商采用适配器接口：创建结账、验证 webhook、取消/恢复订阅、退款。webhook 必须验签、按事件 ID 幂等、记录原始事件并异步处理；客户端回调不能直接改变订阅或余额。充值、退款、赠送和订阅权益分开记账，历史价格和账单不可修改。

## 作业架构与恢复

把当前进程内工作流拆成 `Job` + `JobEvent`。HTTP 请求只创建作业并返回 `202` 与 job ID；worker 从数据库领取带租约的作业，租约过期可重试。状态转换使用版本号条件更新：`queued → running → succeeded/failed/cancelled`，同一版本只能成功一次。

视频、音频和渲染步骤各自成为可重试子作业；云端 provider task ID、幂等键、尝试次数和最后错误写入事件。重试采用指数退避并设置最大次数；不可重试错误进入 dead-letter 状态，管理员可重新排队。进程重启时扫描过期租约和未完成 provider 任务，先查询远端状态再决定继续或释放费用。

生产部署使用 PostgreSQL、对象存储和队列（Redis、SQS 或同类服务）。SQLite 继续作为开发模式。媒体通过短期签名 URL 访问，worker 下载前校验域名、大小、内容类型和 SHA-256；对象存储生命周期负责过期清理，数据库只保存元数据。

## 安全与合规

- 所有 API 统一使用组织上下文中间件；对象级授权检查组织、项目和成员角色。
- 登录增加速率限制、密码强度校验、会话轮换、CSRF 防护和可选 MFA；API key 仅允许 HTTPS 和最小 scope。
- 参考照片、真人素材和导出文件默认私有；下载 URL 最短有效期，审计记录访问者、对象和原因。
- 对上传文件执行大小、MIME、扩展名、病毒扫描和媒体解码隔离；FFmpeg worker 使用无特权容器和资源上限。
- 提供数据导出、组织删除、素材删除和保留策略；审计日志不可由组织管理员删除。
- 明确 AI 数据使用政策：默认不将用户素材用于训练；provider 请求记录模型、版本、区域和数据保留选项。

## 可观测性与运营

每个请求、job、provider 调用和账单都带 `request_id`、`organization_id`、`project_id`。输出结构化日志、指标和 trace，至少监控：队列等待时间、各 provider 成功率/延迟、生成成本、渲染失败率、存储用量、余额冻结时长和 webhook 延迟。

运营后台需要组织检索、成员与权限、订阅状态、作业重试/dead-letter、账本调整（双人复核）、provider 健康、审计导出和成本报表。敏感操作必须填写原因并产生审计事件；支持按组织暂停生成而不影响下载和导出。

## API 约定

新增 `/api/orgs`、`/api/orgs/{id}/members`、`/api/orgs/{id}/invitations`、`/api/orgs/{id}/subscription`、`/api/orgs/{id}/usage`、`/api/jobs/{id}`、`/api/webhooks`。所有写接口支持 `Idempotency-Key`，响应统一包含 `requestId`；异步创建返回 `202 {jobId,state}`，错误使用机器可读 `code`、用户可读 `message` 和可选 `retryAfter`。

## 分阶段落地

1. **隔离基础**：组织/成员表、项目与账本加 `organization_id`、Repository 组织参数、个人组织迁移、对象级授权测试。
2. **作业持久化**：jobs/events、租约 worker、provider 状态恢复、幂等和 dead-letter；单机先用 PostgreSQL 表轮询。
3. **权益与订阅**：entitlements、用量原子扣减、订阅适配器、签名 webhook、账单对账。
4. **生产化**：对象存储、队列、多 worker、签名媒体 URL、限流、MFA、病毒扫描、监控告警和备份恢复演练。
5. **运营闭环**：组织自助服务、邀请与 API key、成本/毛利报表、数据导出删除、SLA 与状态页。

每阶段都应补充 SQLite/ PostgreSQL Repository 契约测试、并发额度测试、跨组织访问拒绝测试、worker 崩溃恢复测试和 webhook 重放测试。完成第二阶段前不要宣称支持多实例；完成第四阶段前不要把用户媒体放入本地磁盘作为生产方案。
