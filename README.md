# 誓光 Vowfilm Studio

可运行的 React + Go 婚礼视频创作工作台。星网 GPT‑6 编排分镜，Seedance 2.0 Fast 生成片段，Go 调用 FFmpeg 合成 1–4 分钟影片。前端展示的是后端真实任务、片段与成片。

本次交付包含 **60 秒、1280 × 720、24 fps 的虚构婚礼演示《把余生写成我们》**，8 个实际生成的 Seedance 镜头、两处 0.5 秒叠化、中文字幕与程序原创钢琴配乐。网页归档为 `public/demo/a-promise-in-light.mp4`。完整导出和原始片段保存在本机 `data/film_demo/`，不会提交到源码仓库。演示前期分镜已生成后按用户新指示切换 GPT‑6，最终梗概及字幕经 GPT‑6 实际审阅修订；后续新项目直接使用 GPT‑6 规划。

演示 MP4 属于生成产物，随交付 ZIP 和网站静态资源分发，不纳入 Git 源码历史。`public/demo/film-manifest.json` 记录其 SHA-256 与媒体规格。单独克隆源码时，可从网站下载成片到 `public/demo/a-promise-in-light.mp4`；在线创作功能不依赖这个演示归档。

## 本地运行

需要 Node.js ≥ 22.13、Go ≥ 1.23、Python 3、FFmpeg/ffprobe（libx264、libass）和 Noto CJK 字体。

```bash
npm ci
cp .env.example .env
# 在 .env 填写 STARNET_API_KEY，并将 GO_BACKEND_TOKEN 改为随机长字符串
bash scripts/dev.sh
```

打开 `http://localhost:5179`。开发脚本同时启动 Go API 和 React，退出时关闭它启动的 Go 进程。若当前机器已运行 `vowfilm-studio.service`，可以只运行 `npm run dev -- --host 127.0.0.1 --port 5179`，使用已配置的 `.dev.vars`。

密钥仅保存在被 Git 忽略的 `.env` 中；`.dev.vars` 只包含服务地址和后端通信令牌。生产部署通过 Sites 的运行时变量配置 `GO_BACKEND_URL`、`GO_BACKEND_TOKEN`。星网密钥只留在 Go 服务端，浏览器不接触密钥。

默认模型和接口：

| 用途 | 配置                                                           |
| ---- | -------------------------------------------------------------- |
| 网关 | `https://open.embervale.cn`                                    |
| LLM  | `openai/gpt-6-astra`，通过 `/v1/chat/completions` 实际调用验证 |
| 视频 | `volcengine/doubao-seedance-2-0-fast-260128`                   |
| 任务 | `POST /v1/videos/generate` → `GET /v1/tasks/{task_id}`         |
| 输出 | 720P、24 fps、H.264 + AAC，16:9 或 9:16                        |

## 使用流程

1. 新建影片，选择 1/2/3/4 分钟、画幅、风格，填写故事。
2. 在素材库上传场景参考、新人照片或配乐。真人素材按当前 Seedance 2.0 接口要求先完成平台授权与入库，再在页面绑定已授权 `asset://asset-…`；服务端验证素材为 `Active`。当前页面不代理第三方的人脸授权流程。没有新人照片时创作虚构人物。
3. 点击「一键生成影片」：自动规划分镜、提交片段、轮询任务、保存素材、剪辑、加字幕、配乐、验收和导出。也可先单独编排分镜。
4. 点击某个镜头可编辑生成指令并局部重做；其他已完成镜头保留。修改项目设置或重新编排会清空当前分镜，旧媒体文件仍保存在磁盘。
5. 暂停后可继续；暂停停止本地轮询，已经提交的云端任务可能继续执行并计费。继续时复用已有任务 ID。传输结果不明时复用幂等键，明确失败后增加尝试编号。

网页归档模式会明确提示后端离线，保留演示片播放、下载和脚本查阅，禁用编辑与生成；不会显示模拟的生成进度。

## 实现与边界

`React → Sites Worker 代理 → Go HTTP API → 星网 GPT‑6 / Seedance → FFmpeg`

- React：工程列表、视频播放器、分镜编辑、素材上传、时间线、进度与制作记录。
- Worker：同源 API 代理、修改请求 Origin 校验、后端令牌注入、媒体 Range 转发。
- Go：持久化状态机、全局 2 路视频提交/轮询、任务恢复、媒体下载、预算上限、剪辑。
- 存储：单进程 JSON 原子写入 + 本地媒体目录，适合单用户 MVP；运行多个 Go 副本前必须改用数据库、任务队列和对象存储。
- 每个工程生成预算是目标成片时长的 3 倍，按提交的视频秒数保守记账；这不是人民币余额或供应商最终账单。单镜头最多 3 次有编号的提交。
- 人物一致性依赖参考素材、统一描述与人工审片，当前没有自动人脸一致性/手部质量模型，也没有自动视觉质检后重生成。不要将技术验收等同于画面语义验收。
- 已实际验证 60 秒横屏端到端流程；120/180/240 秒和竖屏的帧数编排已测试，尚未为这些组合付费生成完整影片。
- 无语音旁白。默认配乐由程序合成，可替换为用户上传的音频。
- 特性检测后注册 WebMCP 的读取工程、创建草稿两个工具；不自动启动付费生成。当前环境没有可用于 WebMCP 的浏览器测试入口，因此未做浏览器调用验收。

当前 Go 服务由本机 `systemd --user` 的 `vowfilm-studio.service` 常驻运行。网站通过 `vowfilm-tunnel.service` 管理的、带令牌的临时 HTTPS 隧道连接本机，隧道重启后需将新地址更新到 Sites 的 `GO_BACKEND_URL`。机器或隧道停止后新生成不可用；已上传网站的演示成片仍可观看。正式运营应将 Go 部署到有持久磁盘的常驻服务器，使用固定 HTTPS 域名替换 `GO_BACKEND_URL`，并备份 `data/`。

## 源码索引

| 文件                                 | 职责                                       |
| ------------------------------------ | ------------------------------------------ |
| `app/page.tsx` / `app/globals.css`   | 中文创作界面、交互与响应式样式             |
| `app/api/[...path]/route.ts`         | Worker → Go 的服务端代理                   |
| `server/internal/studio/provider.go` | GPT‑6 导演 Prompt、终审 Prompt、视频适配器 |
| `server/internal/studio/workflow.go` | 持久化状态机、恢复、预算、并发与素材引用   |
| `server/internal/studio/model.go`    | 工程模型、存储、精确帧数与转场编排         |
| `server/internal/studio/render.go`   | FFmpeg、ASS 字幕、原创配乐、媒体验收       |
| `docs/workflow.md`                   | Prompt 约束、工作流、接口说明              |

## 验证

```bash
npx tsc --noEmit
npm run lint
npm run build
cd server
go test -race ./...
```

Go 测试覆盖 1–4 分钟混合转场的精确时长、JSON 存储隔离与失败回滚、HTTP 鉴权与参数校验、视频任务幂等键/轮询、ASS 转义和配乐文件。Lint 检查业务源码；原样保留的 shadcn 生成组件目录排除在 Lint 外，仍纳入 TypeScript 检查。实际成片另用 ffprobe 验证时长/分辨率/音轨，用 FFmpeg 完整解码检查并抽帧审阅。没有进行浏览器自动化或逐帧人工审片。
