> 状态：已采纳
> 适用范围：当前项目
> 最后更新：2026-08-19

# ADR 0006：使用 MySQL 作为主数据库

## 背景

项目早期使用 PostgreSQL 和 `pgx` 完成关系型数据库访问。当前运行环境统一调整为 Docker MySQL，以保持本地依赖、SQL 方言和项目文档一致。

## 决策

- 主数据库使用 MySQL 8.4.11 LTS。
- Go 通过标准库 `database/sql` 和 `github.com/go-sql-driver/mysql` 访问数据库。
- MySQL 使用 `utf8mb4_0900_ai_ci` 排序规则和 `+08:00` 时区。
- Redis 继续保存在线状态等高频、临时且可重建的数据。
- 不迁移旧 PostgreSQL 测试数据，MySQL 使用 schema 和 seed 重新初始化。

## 兼容约束

- HTTP 路径、JSON 响应、错误码、JWT 和 WebSocket 行为保持不变。
- 封禁、解封与 GM 操作日志继续在同一事务中提交。
- SQL 参数统一使用 `?` 占位符。
- 原有 `RETURNING` 操作改为执行写入后重新查询记录。
- 重复用户名通过 MySQL duplicate-key 错误识别，继续返回原有业务错误码。

## 结果

项目运行依赖变为 MySQL 和 Redis，数据库访问统一使用 `database/sql`。旧 Day 文档保留 PostgreSQL 内容作为历史学习记录，不代表当前运行配置。
