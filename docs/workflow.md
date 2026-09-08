# Prompt 与工作流

实际运行使用有边界的任务状态机；GPT‑6 负责语义创作，Go 负责可验证的造型引用、帧数、任务和媒体处理。没有引入自主无限循环的 Agent。

## 现场方案节点

输入 `occasion`（opening/story/warmup）、事实素材 `brief`、`customPrompt`、`wardrobeMode`（auto/fixed/custom）、`wardrobePrompt` 和可选 `endingText`。完整方案 Prompt 见 `server/internal/studio/treatment.go` 的 `treatmentPrompt`。

GPT‑6 先返回 `Treatment`：concept、identityAnchor、openingHook、closingLine、musicDirection、bpm、mustHave、avoid、notes、looks、acts。Go 验证造型数量、重复 ID、章节引用、桥接枚举、字数与 BPM 60–160，分配连续镜头范围，各章等长分配，章内保留长短镜头变化，再按目标 BPM 量化到帧并计算实际章节起止时间。方案结构不合格时，最多自动修复一次，再失败则停止，不提交视频。固定模式仅允许一套；自动最多三套，自定义最多四套；每章引用一个造型。

该方案先保存，再按每批八镜生成分镜。每批都带上完整方案、用户原始 Prompt、以及每镜的章节与衣服分配，不靠上批短摘要记住造型。编译阶段给每镜写入 identityAnchor 和本章衣服，跨章换装时给前镜尾动作、后镜头动作加入同一遮挡或旋转要求。换装使用独立片段的切接；裁切时优先保留出镜片段的实际结尾及入镜片段的开头，避免统一裁掉遮挡动作；没有在单个生成片段中做衣服形变，也未实现精确首尾帧接力或自动遮挡帧识别。

`customPrompt` 的具体要求优先于风格默认值。实际服装和参考照片的人物身份分别处理。方案说明会标出必要假设和未实现的声音能力，用户可在“导演方案”页查看。修改创作设置清空旧方案、旧分镜与旧配乐任务；暂停恢复保留已保存方案。

## 导演节点

代码中的完整 Prompt 位于 `server/internal/studio/provider.go` 的 `directorPrompt`。

输入包括片名、故事、风格、画幅、目标时长、按风格每分钟 12/14/16/18 个镜头的数量（每批最多 8 镜），以及素材角色/名称。当前 LLM 读取素材描述与名称，照片由视频模型参考；未实现 LLM 图像内容自动分析。

核心指令：

> 只返回 JSON；按情绪推进，交替安排远景、中景与细节。每个镜头只有一个主动作和一种运镜，留出稳定头尾。统一发型、服装、人物方位与光线。至少 80% 自然切接或动作/构图匹配，全片最多两处有理由的叠化或短闪白。不得编造新人未提供的真实经历；字幕由后期添加，生成画面无文字、无音乐。

输出例：

```json
{
  "synopsis": "相遇从一束晨光开始，故事走向并肩的余生。",
  "shots": [
    {
      "title": "并肩花径",
      "chapter": "相伴",
      "description": "新人沿白玫瑰花径缓缓前行",
      "camera": "背后缓慢跟拍",
      "caption": "两个人，同一个方向",
      "transition": "cut",
      "prompt": "成年虚构新人，新娘黑色低盘发、象牙白缎面婚纱，新郎黑色短发、深藏蓝普通西服。两人沿白玫瑰花径缓慢前行，背后中远景跟拍，暖金色自然光。本章造型与左右位置保持一致，单一连续镜头，稳定头尾，无字幕、无文字、无背景音乐。"
    }
  ]
}
```

服务端校验镜头数量、必要文本、指令长度，重置任务字段；LLM 不决定请求地址、鉴权、视频文件路径、预算或任务状态。完整分镜的字幕也可调用 `/review` 由当前 GPT‑6 重新审阅，保留原视频片段并重新合成。

## 帧数与转场

目标帧数 `N = 秒数 × 24`。自然切接的重叠为 0 帧，叠化重叠为 12 帧。片段需分配的总帧数是 `N + 所有转场重叠帧数`，早期工程按整除分配，新的风格先按节奏权重分配净时长，再将绝对切点量化到目标 BPM 节拍，最后加上该镜头的转场重叠。短闪白重叠 3 帧，匹配切接重叠 0 帧；最后一镜补齐目标帧数，因此合成时长不会因转场而缩短。

模型生成时长为 `ceil(镜头剪辑秒数 + 0.75)`，限定 4–15 秒，后期裁切到精确帧数。FFmpeg 将每个片段统一为 24 fps、相同画幅和时间基，再执行 concat / xfade；字幕使用 ASS，不依赖模型生成汉字。

本版实现 cut、match、dissolve、dipwhite 四种转场标记。match 使用自然切接，并将上镜的 exitAction、下镜的 entryAction 和 transitionReason 纳入生成指令；尚未自动检测实际画面的动作/构图匹配质量，也没有首尾帧接力或光流插帧。欢快、旅行、时尚风格将慢叠化改成匹配切接。

## 状态机与故障处理

```text
draft → planning → planned → generating → rendering → completed
                    ↑            ↓            ↓
                    └──── failed / cancelled ─┘
```

- 每次有副作用的提交前保存秒数预算和尝试编号。
- 幂等键包含工程、修订、镜头、尝试：`vowfilm:<project>:r<revision>:<shot>:a<attempt>`。
- 云端返回任务 ID 后持久化；轮询失败不立即重新提交视频。
- 下载只接受配置内已知视频 CDN 的 HTTPS 地址，限制文件大小并拒绝重定向。
- 已完成片段经 ffprobe 检查后留在磁盘；继续生成跳过已完成镜头。
- 合成使用本地 FFmpeg，失败可仅重新合成，无须再次调用视频模型。
- 导出 `quality-report.json` 记录技术检查结果；人物身份验证明确为 `not_automated`。

## HTTP 接口

浏览器通过同源 `/api` 访问。直接调用 Go 时必须携带 `X-Vowfilm-Token`；`/healthz` 无须认证，提供给配乐服务的单个媒体文件可使用限时签名读取。

| 请求                                            | 行为                                           |
| ----------------------------------------------- | ---------------------------------------------- |
| `GET /api/config`                               | 模型名、连接配置、时长上限和并发数；不返回密钥 |
| `GET/POST /api/projects`                        | 列表 / 创建工程                                |
| `GET/PATCH /api/projects/{id}`                  | 读取 / 修改创作设置并重置分镜                  |
| `POST /api/projects/{id}/plan`                  | 仅编排分镜                                     |
| `POST /api/projects/{id}/generate`              | 一键生成或继续                                 |
| `POST /api/projects/{id}/review`                | 当前 LLM 审阅梗概/字幕，保留片段再合成         |
| `POST /api/projects/{id}/render`                | 仅重新合成                                     |
| `POST /api/projects/{id}/cancel`                | 暂停本地制作任务                               |
| `POST /api/projects/{id}/assets`                | multipart `file` + `role` 上传                 |
| `PATCH /api/projects/{id}/assets/{asset}`       | 绑定 `providerAssetId`                         |
| `PATCH /api/projects/{id}/shots/{shot}`         | 修改 `prompt`                                  |
| `POST /api/projects/{id}/shots/{shot}/generate` | 局部重做并合成                                 |
| `GET /api/projects/{id}/export`                 | 下载 JSON 工程                                 |
| `GET /api/media/{id}/{filename}`                | 媒体播放，支持 Range；`?download=1` 下载       |

本次接口依据星网网关实际文档及调用结果实现。不同 Seedance 型号在实名授权、参考图和首尾帧方面可能有不同约束，切换型号时需要重新验证适配器。


## AI 配乐节点

剪辑画面就绪后，Go 生成仅允许读取该视频、一小时过期的 HMAC 签名 URL，再向星网 `POST /v1/video-to-music` 提交 multipart：`video_url`、`mode=async`、`output_format=mp3`、`variants_num=1`、`prompt_influence=1` 与风格化配乐 Prompt。

Prompt 要求一首连贯的原创器乐：0–13% 简洁引子，13–40% 主旋律与轻节奏，40–67% 相对小调或对比和声的桥段与节奏留白，67–87% 完整鼓组和主旋律变奏的高潮，87–100% 旋律回归与明确终止；不使用单一琶音从头循环到尾。六种风格传入不同的乐器、律动与音乐类型。

任务 ID 持久化后通过 `/v1/tasks/{id}` 轮询。原生响应中的 `audio[].url` 可能是归档素材对象，需提取 `asset_id` 并下载网关 CDN 中的音频。任务采用包含工程与修订号的幂等键；重做单个镜头时可保留现有配乐。音频混音统一为 48 kHz 双声道 AAC，配合短淡入淡出。

五段结构是配乐设计目标；当前未自动识别实际音乐拍点，也未强制变速将每个生成拍点与视觉切点对齐。用户可在网页单独试听音乐并上传替换音轨。

## 现场片尾

opening 保持最后三秒画面并在此前两秒淡出音乐，story 保持两秒；片尾大字显示至最后一帧。字幕在冻结画面之后叠加，保证结束提示可见。warmup 不冻结、不调用入场提示，网页播放器设置循环。横屏使用更大的中文正文字号与安全边距；任意 LED 异形屏分辨率还需按场地实际规格另行适配。
