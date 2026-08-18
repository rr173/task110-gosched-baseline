# task110-gosched — Go 任务调度引擎

## 项目用途

`task110-gosched` 是一个**通用的 Go 任务调度引擎**，对外提供 HTTP API 来创建、查询、启停与触发定时/一次性/延迟任务，并以 SQLite 做真实持久化（含重启恢复）。调度器后台循环扫描到期任务、生成执行记录（run）、按策略重试失败任务，并将执行结果、统计与标签落库。它属于技术基础设施组件，不是业务平台系统。

核心能力：

- **三种调度类型**：`cron`（标准 5 段 cron 表达式）、`once`（指定时间一次性）、`delayed`（相对当前延迟 N 秒）。
- **执行闭环**：调度 → 生成 run → 执行 → 落库结果 → 更新任务下次运行时间；cron 任务自动推进 `next_run_at`。
- **真实持久化与重启恢复**：任务、run、标签、统计均写入 SQLite（WAL）；进程崩溃后，处于 `running` 的孤立 run 会被标记为 `orphaned` 并按重试预算重投。
- **重试策略**：失败 run 在 `max_retries` 预算内按退避重投，退避由已尝试次数决定。
- **标签与统计**：任务可打多标签，支持按标签过滤、批量暂停；提供整体与按日统计。

## 标准构建 / 运行 / 测试命令

编译（使用 Go module 模式，CGO 关闭）：

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
```

启动（默认监听 :8080，SQLite 文件 gosched.db）：

```bash
GOTOOLCHAIN=local go run . --addr :8080 --db gosched.db
```

运行测试：

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go test ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet ./...
```

自检（不依赖外部服务，执行后自行退出）：

```bash
GOTOOLCHAIN=local go run . --smoke-test
```

`--smoke-test` 会在临时 SQLite 上跑完整契约：创建 cron/once/delayed 任务、触发执行、失败重试、跨重开的持久化与恢复、统计与标签，全部通过即退出 0，失败则退出非 0。

## Benzhi Docker 构建

```bash
bash build_benzhi_docker.sh gosched linux/amd64
# 双架构再验证：
bash build_benzhi_docker.sh gosched linux/arm64
```

镜像基于固定 `golang:1.26.3`，依赖由构建阶段自动下载。容器启动后进入 shell，可手动 `go run . --smoke-test` 验证。

## 主要 API（节选，完整见 internal/api）

- `POST   /api/v1/tasks`            创建任务
- `GET    /api/v1/tasks`            列出任务（支持 tag/status/enabled/paused 过滤、分页）
- `GET    /api/v1/tasks/{id}`       获取任务
- `PUT    /api/v1/tasks/{id}`       更新任务
- `DELETE /api/v1/tasks/{id}`       删除任务
- `POST   /api/v1/tasks/{id}/enable`   启用
- `POST   /api/v1/tasks/{id}/disable`  停用
- `POST   /api/v1/tasks/{id}/trigger`  立即触发一次
- `POST   /api/v1/tasks/{id}/pause`    暂停调度
- `POST   /api/v1/tasks/{id}/resume`   恢复调度
- `GET    /api/v1/tasks/{id}/runs`     列出某任务的 run
- `GET    /api/v1/runs`               列出所有 run
- `GET    /api/v1/runs/{id}`           获取 run
- `POST   /api/v1/runs/{id}/retry`     重试失败 run
- `DELETE /api/v1/runs/{id}`           删除 run 记录
- `GET    /api/v1/tags`               列出标签
- `GET    /api/v1/tags/{tag}/tasks`   按标签查任务
- `POST   /api/v1/tags`               创建标签
- `GET    /api/v1/stats`              整体统计
- `GET    /api/v1/stats/daily`        按日统计
- `GET    /api/v1/schedules/next`     批量计算下次运行时间
- `POST   /api/v1/tasks/bulk-pause`   按标签批量暂停
- `GET    /api/v1/health`             健康检查
- `GET    /api/v1/metrics`            运行时指标
