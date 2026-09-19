# 文档索引

日常使用从项目 [README](../README.md) 开始；开发定位从 [系统架构](architecture.md) 开始。当前实现以代码和下列使用说明为准。

## 当前功能与运行

| 文档                                      | 内容                                                    |
| ----------------------------------------- | ------------------------------------------------------- |
| [系统架构](architecture.md)               | 功能入口、目录职责、三条生成链路、状态与媒体边界        |
| [婚礼故事模板](wedding-story-template.md) | 一键成片、14 步制作、原版 Prompt、Python 依赖与演示交付 |
| [模板与 Agent](template-agents.md)        | 固定婚庆模板、故事模板和自由创作的区别                  |
| [电商广告](advertising-prompts.md)        | 10–60 秒、生成分组、人物参考、口播与原生声音            |
| [婚礼现场制作](wedding-production.md)     | 入场、爱情回顾、暖场用途及造型要求                      |
| [通用工作流](workflow.md)                 | 导演、分镜、配乐与 FFmpeg 合成                          |
| [后端与数据库](backend-architecture.md)   | Go 分层、SQLite/PostgreSQL、迁移与单实例边界            |
| [计费模型](platform-model.md)             | 报价、冻结、结算、权限与账本                            |
| [视觉主题](visual-theme.md)               | 已实现的品牌、颜色与页面主题                            |

## 设计与历史记录

这些文件保留方案背景，不能作为功能已上线的依据。

| 文档                                                     | 定位                                                     |
| -------------------------------------------------------- | -------------------------------------------------------- |
| [婚礼向导初始评估](wedding-guided-wizard-integration.md) | 接入前的上游核对与方案；实际入口和执行方式见婚礼故事模板 |
| [SaaS 平台化设计](saas-platform-design.md)               | 组织、订阅、队列、对象存储等后续方案                     |
| [场景化产品设计](uiux-business-design.md)                | 创建向导和未来模板库设计，含尚未实现条目                 |
| [设计参考](../design-system/vowfilm-studio/MASTER.md)    | 早期生成的视觉建议；实际 token 以 `app/globals.css` 为准 |

## 原版材料与交付物

- `server/internal/wedding/upstream/`：原版婚礼文件，只更新经过核对的版本；`source.json` 保存逐文件摘要。
- `server/internal/advertising/prompts/`：广告 Prompt 原文与来源清单。
- `public/templates/wedding-story-intake.html`、`wedding-story-LICENSE.txt`：可直接打开的原版采集卡及许可证。
- `public/demo/`：模板预览；MP4/MP3 按现有约定单独分发。
- `deliverables/`、`data/`：本地生成成果与运行数据，不提交到 Git。
