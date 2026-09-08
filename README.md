# 誓光 Vowfilm Studio

React + Go 婚礼视频工作台：风格选择、GPT‑6 分镜、SD2 Mini 视频生成、动作衔接、按节拍剪辑、AI 配乐、局部重做与成片下载。

默认 LLM：星网 `openai/gpt-6-astra`。默认视频模型：**SD2 Mini**，完整标识 `volcengine/doubao-seedance-2-0-mini-260615`。

## 交付影片

《与你，快乐加倍》是 60 秒、1280 × 720、24 fps 的欢快婚礼演示。16 个实际生成的 SD2 Mini 镜头由 GPT‑6 编排，从回头相见、牵手起跑到转圈、抛花瓣、碰杯与拥抱。镜头为 2–4 秒，切点按 120 BPM 节拍网格安排。星网 Sonilo 根据画面和五段音乐指令生成配乐。

影片使用虚构成年人物，未使用真实新人照片。新版归档为 `public/demo/joy-together.mp4`；旧版在 `public/demo/a-promise-in-light.mp4` 供对照。当前工程、原始片段、完整成片和质量报告保存在本机 `data/`。

MP4 属于生成产物，随交付 ZIP 和网站资源分发，不进入 Git 历史。`public/demo/film-manifest.json` 记录 SHA-256 和媒体规格。单独克隆源码时可从交付包补充演示文件；在线创作不依赖演示归档。

已完成工程可用 `python3 scripts/export-demo.py 工程ID --name joy-together` 重新导出演示归档；脚本压缩影片、验证完整解码、复制配乐和分镜缩略图，并清理云端任务标识。

## 风格影响实际生成

| 风格 | 每分钟镜头 | 目标节奏 | 内容与音乐方向 |
| --- | --- | --- | --- |
| 欢快庆典 | 16 | 120 BPM | 笑闹与庆祝；吉他、主旋律、贝斯、拍手与鼓组 |
| 浪漫电影 | 12 | 96 BPM | 亲密互动与眼神；抒情钢琴、拨弦、渐进弦乐 |
| 复古胶片 | 14 | 108 BPM | 暖调抓拍；摇摆爵士、低音提琴与刷鼓 |
| 史诗仪式 | 12 | 96 BPM | 空间与仪式高潮；管弦乐、打击乐、终章回落 |
| 旅行纪实 | 16 | 120 BPM | 连贯移动与旅途互动；原声民谣、拍手节奏 |
| 时尚短片 | 18 | 120 BPM | 干净构图与利落切镜；Nu-disco、律动贝斯 |

60 秒影片的配乐结构为：开场 0–8 秒、推进 8–24 秒、转折 24–40 秒、高潮 40–52 秒、收尾 52–60 秒。长片按比例扩展。AI 配乐接收这些目标，具体拍点和和声仍需审听；当前没有自动音乐拍点识别或音频时间拉伸。

## 本地运行

需要 Node.js ≥ 22.13、Go ≥ 1.23、Python 3、FFmpeg/ffprobe（libx264、libass）及 Noto CJK 字体。

```bash
npm ci
cp .env.example .env
# 填写星网密钥、随机后端令牌和配乐所需素材服务地址
bash scripts/dev.sh
```

打开 `http://localhost:5179`。脚本同时启动 Go 与 React，退出时关闭它启动的 Go。当前机器已运行 Go 服务，可只运行 `npm run dev -- --host 127.0.0.1 --port 5179`，避免重复占用 Go 端口。

验收后的构建可用 `npm run build && npm start` 在同一地址运行；Go 服务需另行保持运行。`npm start` 从本机 `.dev.vars` 读取网页代理配置，构建产物不包含密钥。

| 配置 | 作用 |
| --- | --- |
| `STARNET_BASE_URL` | 默认 `https://open.embervale.cn` |
| `STARNET_API_KEY` | 星网密钥，只留在 Go 服务端 |
| `STARNET_LLM_MODEL` | 默认 GPT‑6 Astra |
| `STARNET_VIDEO_MODEL` | 默认 SD2 Mini |
| `GO_BACKEND_URL` | 网页服务端连接 Go，本地为 `http://127.0.0.1:8097` |
| `GO_BACKEND_TOKEN` | 至少 24 字符的随机后端令牌 |
| `PUBLIC_MEDIA_BASE_URL` | Go 素材服务的 HTTPS 地址，供 AI 配乐读取视频 |
| `VOWFILM_CONCURRENCY` | 1–6 路任务，默认 2，本次使用 4 |

`.env` 和 `.dev.vars` 排除出 Git 与交付包。新风格需要可访问的 `PUBLIC_MEDIA_BASE_URL` 才能自动配乐；也可上传自己的音频。配乐服务缺失时不会悄悄换回循环伴奏。

## 使用

1. 新建影片，选择风格、1/2/3/4 分钟和画幅，填写故事与场景。
2. 上传参考图片或音乐。真人素材需先在第三方平台完成授权与入库，再绑定已授权 `asset://asset-…`；服务端验证其为 `Active`。网页不代替第三方完成本人授权。
3. 一键生成，或先编排分镜。点击镜头查看入场、出场和衔接理由，编辑指令后局部重做。
4. 查看配乐段落、单独试听音轨，播放与下载成片，导出 JSON 工程。
5. 暂停后可继续；已经提交的云端任务可能继续计费。恢复时复用任务 ID，保留已完成镜头。修改风格或重新编排会重建当前分镜，原始文件仍在磁盘。

## 工作流

`React → Sites Worker 同源代理 → Go 状态机 → 星网 GPT‑6 / SD2 Mini / Sonilo → FFmpeg`

- GPT‑6 每批最多编排 8 镜，传递前批结尾与全片位置，减少长 JSON 请求超时。Go 校验数量和字段。
- 风格决定内容、镜头数量、运镜、目标 BPM 和音乐方向；Go 计算变化的镜头时长并补齐转场重叠帧。
- 动作/构图切接为主，最多两处叠化或短闪白。欢快、旅行、时尚风格不采用慢叠化。匹配依赖生成指令，目前没有自动视觉匹配检测或光流转场。
- FFmpeg 统一画幅与 24 fps，烧录中文标题和字幕、混音并验收时长。
- Sonilo 读取一小时有效、限定单个文件的签名视频 URL；签名不授予修改权限。
- 单进程 JSON 原子保存、本地媒体、秒数预算、幂等提交、恢复与 CDN 下载校验，适合单用户 MVP；多人生产需数据库、队列和对象存储。
- 视频生成预算为成片时长的 3 倍，每镜最多 3 次有编号提交。这不是人民币账单，不包含 LLM 与音乐费用。

`server/internal/studio/` 下：`provider.go` 包含完整 Prompt；`style.go` 是风格库；`model.go` 编排帧数；`workflow.go` 管理任务；`soundtrack.go` 生成配乐并签名素材；`render.go` 剪辑与排字幕。详见 [工作流说明](docs/workflow.md)。

## 验证与边界

```bash
npx tsc --noEmit
npm run lint
npm run build
cd server
go test -race ./...
```

测试覆盖六种风格在 1–4 分钟的时长与节拍网格、重叠帧、存储回滚、鉴权、幂等、签名资源范围和过期、原生音频归档解析、字幕转义。Lint 检查业务代码，原样保留的 shadcn 组件仍接受 TypeScript 检查。

实际验证 60 秒横屏成片；其他时长与竖屏通过编排测试，未逐一付费生成完整影片。成片检查完整解码、音视频轨道并抽帧审阅。没有自动人脸或手部质量检测，不保证每次人物服装和动作完全一致。当前环境未进行浏览器自动化或 WebMCP 浏览器调用验收。

当前 Go 和素材隧道由本机 `vowfilm-studio.service`、`vowfilm-tunnel.service` 常驻运行。机器或隧道停止后无法新生成；隧道重启后需更新网页连接地址与 `PUBLIC_MEDIA_BASE_URL`。正式运行请使用固定 HTTPS 服务器并备份 `data/`。
