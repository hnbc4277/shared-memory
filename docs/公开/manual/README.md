# Shared Memory 文档导航（新手从这里开始）

本项目是**自托管**的「共享记忆」平台：多种智能体共用同一账号下的历史、记忆、凭证、成稿与知识库。

| 文档 | 适合谁 | 内容 |
|------|--------|------|
| [系统设计](../design/SYSTEM-DESIGN.md) | 想理解整体的人 | 目标、架构图、模块、数据流、安全模型 |
| [部署指南](../deploy/DEPLOYMENT-GUIDE.md) | 第一次装服务器的人 | 环境、Compose、构建、HTTPS、验收 |
| [迁移指南](../migration/MIGRATION-GUIDE.md) | 升级版本的人 | SQL 迁移顺序、数据/索引迁移、回滚 |
| [操作手册（图文版）](OPERATIONS-MANUAL.md) | 日常运维/使用的人 | 珍珠玻璃 UI：**总览 Bento · Ctrl+K · 各 Tab**；见 [images/README.md](images/README.md) 重截图 |
| [Cursor 最稳接法](CURSOR-STABLE-SETUP.md) | 要把 Cursor 接到某条正式连接的人 | 钩子自动写入，MCP 负责召回 |
| [图文手册.html](图文手册.html) | 需要看图的人 | 浏览器打开（推荐）；含总览 / 命令面板 / 各业务页 |
| [全功能索引](FEATURE-INDEX.md) | QA / 验收 | **130+** 功能点：S / O / H / A … 编号与图号 |
| [测试矩阵](../../不公开/TEST-MATRIX.md) | QA/发布前检查（内部） | 功能点逐项测试结果；公开仓可不收录 |
| [DEPLOY.md](../DEPLOY.md) | 快速清单 | 精简版上线检查 |
| [API.md](../API.md) / [openapi.yaml](../openapi.yaml) | 集成开发 | HTTP 路由索引 |

**推荐阅读顺序：** 系统设计 → 部署指南 → 操作手册 → 迁移指南（升级时再看）。
