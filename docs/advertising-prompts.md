# 电商广告 v3：单任务直出15秒

来源为用户提供的 `ad-video-prompt-v3.zip`，归档 SHA-256：`73a42c493c895b4c69b064233d3bdead526a145dfc704621b550fbb4d002921c`。

原始 `SKILL.md`、`references/ecommerce.md`（H3 带货分镜生成器）、`references/patterns.md` 和 agent 元数据保存在 `server/internal/advertising/prompts/`，文件哈希在 `source.json`。前三份文件完整嵌入导演系统指令，模板版本按三份内容共同计算（见 `advertising.Version()` 与 `prompt_test.go`）。源码编译后不依赖本机 ZIP 路径、数据库类型或历史租户资料。

## 生成链路

1. 当前商品资料、自定义要求与实际图片顺序进入一次文本模型调用，生成一条符合 H3 生成器结构的完整中文 Prompt（素材绑定 → 整体设定 → `镜头N[MM:SS-MM:SS]` 时间轴 → 段尾 → 约束，含台词/秒与换品自检）。JSON 外层仅服务工程保存与界面展示。
2. 电商不再调用分章造型方案，不拆成三个独立片段，不追加“一镜到底、无音乐”等旧约束。商品图约束外观，人物与场景按各自素材职责使用。文本导演未实际识别图片，不把文件名当成视觉证据。
3. 完整 Prompt 通过一次 `/v1/videos/generate` 提交，`duration=15`、`generate_audio=true`，画幅使用工程配置。默认竖屏，输出720P，内部切镜由视频模型完成。使用管理员已配置的视频模型，未强行切换供应商。
4. 校验15秒时长（画面容许1帧末帧余量，容器容许0.15秒编码尾部余量，音轨至少14.85秒）、目标分辨率及原生音轨。默认直接复制源 MP4：不拼接、不裁剪余量、不替换声音、不自动加字幕/片名、配乐或淡入淡出。用户明确填写片尾大字时仅重编码画面添加覆盖文字，音轨仍流复制。模型字幕由完整 Prompt 安排，其逐字准确性需要观看验收。

工程记录 `generationMode=commerce-direct-15s-v3` 和 `promptPolicy`。旧工程保留播放和导出；继续旧版多片段生成前，需将时长保存为15秒并重新编排。不会把旧分镜静默当成新版任务提交或直接收费。其他场景继续原有多镜头工作流。

## 数量与计费

- 电商新建/修改仅允许15秒；一个工程脚本单元承载整条广告，其内部切镜不再分别计数。
- 按次/按任务：数量1；按秒：15；按镜头生成单元：1。重新编排旧版三镜工程时也按新计划数量1报价。
- 继续复用价格版本、场景系数、报价快照、额度预留、成功结算和失败释放。不会因为15秒任务包含四次切镜而按四个上游任务计费。
- 正常工程累计生成预算45秒，最多三次15秒尝试；沿用幂等任务恢复。已发生的生成量不会因重新编排而清零。

声音来自视频模型。上传音乐不绑定为 `<音频N>`，也不覆盖电商原生声音；非法图片/音频引用在提交前报错。口型、台词准确度和商品结构仍需实际视频验收，不能用“存在音轨”代替听辨。

## 验证

`go -C server test ./...` 覆盖：完整v3来源进入系统指令、一次文本调用和一次15秒有声视频请求、图片索引、旧版拦截、四种计费单位、原生文件逐字节保留、显式片尾覆盖后音频包哈希不变、缺音轨或时长不足拒绝。测试模型由本地HTTP模拟，媒体保留测试由本地FFmpeg生成测试信号，不会产生上游费用。

真实验收入口 `TestLiveCommerceVideo` 默认跳过；只有显式开关才调用付费上游。使用独立文件存储和虚构商品参考，复用生产导演、视频提交/恢复/下载与媒体输出代码，不调整平台钱包、不自动加入账号工程列表。重复运行恢复同一任务；单次验收预算15秒，不自动重生成。

```bash
VOWFILM_LIVE_COMMERCE=1 \
VOWFILM_LIVE_OUTPUT_DIR="$PWD/deliverables/commerce-prompt-v3-test" \
VOWFILM_LIVE_PROVIDER_DIR="$PWD/data" \
VOWFILM_LIVE_PRODUCT_IMAGE="$PWD/deliverables/commerce-prompt-test/fictional-product.png" \
go -C server test ./internal/studio -run '^TestLiveCommerceVideo$' -count=1 -v -timeout 25m
```

新版样片为虚构三格收纳盒的真人手部使用演示，15秒、9:16，在一次任务内完成翻找、分格放置和取用，包含连续女性旁白。旧样片位于 `deliverables/commerce-prompt-test/`，新版单独保存在 `deliverables/commerce-prompt-v3-test/`。
