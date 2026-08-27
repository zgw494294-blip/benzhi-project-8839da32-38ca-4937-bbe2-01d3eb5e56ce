# 舞台吊挂系统年度安全检验工作台

本项目面向剧场设备检验员与舞台机械复核负责人，将吊杆、卷扬机和安全装置的年度检验组织为可追溯闭环：任务建档、设备范围锁定、方案确认、实测采集、自动缺陷评定、整改复验、专业复核、证据冻结、启用许可签发与验真。

服务由 Go 提供响应式浏览器页面与同源 JSON HTTP API，不依赖 Node。业务数据保存在本地 SQLite；数据库启用 WAL，所有写操作使用 `expectedVersion` 乐观并发控制和 `idempotencyKey` 幂等保护。设备与实测批次在单个短事务内原子保存且任务版本只递增一次；实测修订、整改复验轮次和冻结证据不会被原地覆盖，完整决定可通过审计时间线查询。

工作台现支持设备范围批量登记、方案适用性矩阵预检、按设备检查清单批量实测、仪器校准时效汇总、多轮失败复验、结构化复核退回与补证销项、冻结候选清单摘要确认，以及按许可编号和设备编号逐项验真。方案和冻结操作都要求回传预览摘要，防止预览后范围或内容漂移。

## 构建

```bash
go build ./...
```

## 运行

```bash
go run ./cmd/server -addr=127.0.0.1:19081
```

浏览器访问 `http://127.0.0.1:19081/`。默认数据库为当前目录的 `rigging-inspection.db`，可通过 `-db` 指定路径。监听地址也可通过 `PORT` 设置端口号，服务会绑定到 `127.0.0.1:<PORT>`；出于检验数据保护考虑，不接受非回环监听地址。

执行完整、真实 HTTP 回环监听的有界自检：

```bash
go run ./cmd/server -addr=127.0.0.1:19473 -selfcheck -selfcheck-timeout=20s
```

selfcheck 使用临时 SQLite 数据库，经公开 API 依次完成建档、批量设备登记、方案预检与摘要锁定、带校准快照的实测、自动缺陷、整改复验、送审、批准、冻结候选预览、摘要冻结、许可签发、范围验真和审计时间线检查，完成后自行关闭监听并退出。

## 测试

```bash
go test ./...
```

测试覆盖状态机与冻结摘要、阈值和完整度规则、SQLite 重启持久化、版本冲突、幂等重放，以及浏览器入口和 JSON API。

## 主要 API

- `POST /api/v1/campaigns` 创建年度任务。
- `POST /api/v1/campaigns/{id}/assets` 通过 `assets` 数组原子登记设备批次，并返回类别汇总和最新版本；兼容单个 `asset` 请求。
- `POST /api/v1/campaigns/{id}/plans/preflight` 返回适用矩阵、全部问题和确定性摘要；`POST .../plans/confirm` 回传 `previewDigest` 后锁定方案。
- `GET /api/v1/campaigns/{id}/assets/{assetID}/checklist` 获取设备适用清单；`POST .../measurements` 通过 `measurements` 数组原子提交不可变实测修订并自动评定。
- `POST /api/v1/campaigns/{id}/defects/{defectID}/remedy` 与 `.../retest` 完成缺陷闭环。
- `POST /api/v1/campaigns/{id}/review/submit` 与 `.../decision` 完成送审、结构化退回项销项或批准。
- `GET /api/v1/campaigns/{id}/freeze/preview` 获取冻结候选清单；`POST .../freeze` 回传 `candidateDigest` 后冻结规范证据。
- `POST /api/v1/campaigns/{id}/permit` 签发启用许可。
- `GET /api/v1/permits/{number}/verify?assetCode=DG-01` 重算冻结摘要，核对关联与完整范围，并可精确验真单台设备。
- `GET /api/v1/campaigns/{id}/timeline` 查询连续审计时间线。

所有写请求均应携带 `actor`、`role`、`idempotencyKey`；除建档外还需携带当前 `expectedVersion`。实测及复验同时携带 `instrumentCode`、`instrumentCalibratedOn`、`instrumentValidUntil` 和 `measuredAt`。检验员角色为 `inspector`，复核负责人角色为 `reviewer`。
