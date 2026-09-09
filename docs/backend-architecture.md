# 后端分层与数据库切换

## 依赖边界

- `cmd/server`：进程配置与 HTTP 服务启动。
- `internal/studio/http.go`、`auth.go`、`platform.go`：HTTP 参数解析、认证、权限与业务调用。不包含 SQL。
- `internal/studio/workflow.go`：现有影片工作流应用服务，编排模型服务、媒体处理、任务计费与项目仓库。
- `internal/platform`：账号应用服务、权限模型、报价模型和 Repository 接口。不导入 `database/sql`、SQLite 或 PostgreSQL 驱动。
- `internal/domain`：Project、Shot、Treatment 等领域类型及 ProjectRepository 接口。
- `internal/storage`：SQL 仓库实现、事务、参数绑定与数据库迁移。SQLite 使用 modernc 驱动，PostgreSQL 使用 pgx。SQL 方言差异只在这一层。
- `internal/storage/migrations/001_initial.sql`：两种数据库共用的首版 schema，由程序嵌入，在事务中执行；`schema_migrations` 记录版本，拒绝启动在未来版本的数据库上。后续 schema 变化需添加新编号迁移及升级分支，不能修改已应用迁移。

业务模型不依赖具体数据库。开发默认 SQLite；设置数据库驱动与地址即可连接 PostgreSQL。账号、会话、报价、任务账单、积分流水、审计和项目元数据均进入所选数据库。API 密钥配置暂存忽略提交的 `provider.json`；视频/图片/音轨保存在 VOWFILM_DATA_DIR，不写入数据库。

## SQLite 开发配置

```dotenv
DATABASE_DRIVER=sqlite
DATABASE_URL=
```

空地址默认为 `data/platform.sqlite`。文件权限 0600，WAL、外键和 FULL 同步；首次启动导入旧 `projects.json`（仅在项目表为空时），原文件保留。旧项目首次初始化时分配给管理员，不公开给普通注册账号。

## PostgreSQL 配置

```dotenv
DATABASE_DRIVER=postgres
DATABASE_URL=postgres://vowfilm:YOUR_PASSWORD@127.0.0.1:5432/vowfilm?sslmode=require
```

新空库会自动建表。修改配置不会自动搬运原 SQLite 数据，需要下面的迁移命令。部署要求 Go ≥ 1.26（当前数据库与密码库版本要求）。数据库需由部署者创建并授予表/索引创建权限；生产连接按环境配置 TLS。

## 迁移已有数据

1. 停止本项目 Go 进程，备份整个 `data/` 目录；SQLite 使用一致性备份或停止后连同 WAL 文件一起保存。
2. 准备一个空的 PostgreSQL 数据库，通过环境变量提供连接串，避免把密码写入 shell 历史或 Git。
3. 在 `server` 目录运行：

```bash
# 先在当前进程环境安全设置 MIGRATE_TARGET_URL
# MIGRATE_TARGET_URL=postgres://...
go run ./cmd/migrate-db --source-driver sqlite --source-url ../data/platform.sqlite --target-driver postgres
```

4. 工具在单个目标事务中复制用户、会话、价格历史、报价、账单、流水、审计及项目。目标库已有业务记录时拒绝覆盖，失败回滚。源库保持可用；限流窗口无需迁移。
5. 修改 `.env` 的 `DATABASE_DRIVER` / `DATABASE_URL` 后启动。媒体目录保持原位置或完整迁移并设置 `VOWFILM_DATA_DIR`。检查用户数量、项目归属、账本余额和媒体下载后再恢复使用。源库保持停止，避免双写。

迁移也支持 PostgreSQL → SQLite，用 `MIGRATE_SOURCE_URL` 指定源连接、`MIGRATE_TARGET_URL` 指定目标文件。

## 事务与运行边界

余额变更、冻结、结算与流水原子提交。充值凭据、用户幂等键和报价使用数据库唯一约束防重。PostgreSQL 仓库以事务级 advisory lock 串行化关键写入；SQLite 保留单连接事务。当前仍为**单 Go 实例**，影片工作流使用进程内状态和缓存，项目快照原子写入所选数据库。切换 PG 不代表已支持多实例；下一阶段需把任务调度改为队列、项目更新改为行级版本检查，并加入 outbox。

账户/账本与项目快照是不同事务。崩溃恢复采用保守策略：释放所有未结算冻结，暂停项目，保留可恢复的云端任务 ID，由用户再次确认后继续，可能由平台承担已产生的外部成本。

## 测试

```bash
go test -race ./...
# 专用空测试 PG 库；不可使用业务数据库
TEST_POSTGRES_URL=postgres://... go test -race ./internal/storage -run TestRepositoryContract -count=1
```

同一仓库契约验证 SQLite 与 PG 的初始化、账号、角色撤销、凭据防重、价格版本、冻结/结算/释放、并发余额约束、账本一致性和项目保存。PostgreSQL URL 未设置时该分支明确跳过。
