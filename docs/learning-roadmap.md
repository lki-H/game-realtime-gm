# 项目驱动学习计划：从 Go 实时游戏服务项目建立后端工程能力

## 这份文档的作用

这份文档不是某一天的任务清单，而是整个项目学习过程的长期参考。

你的目标不是简单把代码敲完，而是借这个项目掌握一个后端工程师在真实工作中需要的能力：

- 看懂项目结构
- 理解 HTTP 请求流转
- 使用 Go 编写后端服务
- 使用 PostgreSQL 保存长期数据
- 使用 Redis 处理实时状态
- 使用 Docker Compose 复现开发环境
- 使用 JWT 做登录身份识别
- 使用 WebSocket 做实时通信
- 使用测试、日志、压测、pprof 验证服务质量
- 能把项目讲清楚，写进简历，并应对面试追问

## 总体学习方法

不要先把所有基础学完再做项目，也不要只照着项目敲代码。

推荐使用“项目主线 + 基础补洞 + 工程复盘”的学习方式。

```text
1. 先知道这个功能解决什么问题
2. 再看项目里哪些文件参与
3. 照着实现最小可运行版本
4. 跑通接口或命令
5. 回头逐行理解代码
6. 画出调用链或数据流
7. 写一段“我怎么向面试官解释”
```

每做一个功能，都要问自己：

```text
这个功能在真实项目里为什么需要？
它改了哪些文件？
每个文件负责什么？
请求从哪里进来？
数据保存在哪里？
失败时会报什么错？
怎么验证它真的成功？
我能不能用自己的话讲出来？
```

## 三条学习线

### 1. 项目主线

项目主线就是每天的 `dayxx-plan.md`。

它负责告诉你当天要做什么、怎么做、改哪些文件、怎么验证。

你每天优先完成项目主线，因为项目是把所有技术串起来的训练场。

### 2. 基础补洞线

基础补洞线不是让你漫无目的看完所有教程。

正确做法是：项目中遇到什么，就补什么。

例如：

```text
写 config.go 时补 Go struct、package、环境变量。
写注册接口时补 Gin JSON、HTTP 状态码、bcrypt、SQL INSERT。
写登录接口时补 JWT、token、claims、过期时间。
写 WebSocket 时补长连接、心跳、连接生命周期。
写排行榜时补 Redis ZSet。
```

### 3. 工程复盘线

每完成一个功能，都要写一小段复盘。

复盘不是写流水账，而是把功能讲清楚。

例如注册接口完成后，应该能写：

```text
注册接口接收 username、password、nickname。
router.go 把 POST /api/register 转交给 AuthHandler.Register。
handler 使用 Gin 解析 JSON，并用 bcrypt 生成 password_hash。
然后通过 pgxpool 执行 INSERT INTO players。
players 表通过 UNIQUE 约束保证 username 不重复。
注册成功返回玩家公开信息，不返回 password_hash。
```

这段复盘以后就是简历和面试表达素材。

## 每天学习节奏

每天建议分成三段。

### 第一段：做项目

按照当天 `dayxx-plan.md` 完成任务。

重点是把最小闭环跑通。

例如 Day 02 的闭环是：

```text
Docker PostgreSQL 启动
        ↓
Go 后端连接数据库
        ↓
players 表存在
        ↓
注册接口能写入数据
        ↓
登录接口能返回 JWT
```

### 第二段：补基础

只补当天用到的知识点。

不要因为今天用了 PostgreSQL，就立刻去看完整本数据库教程。

你只需要先掌握当天用到的内容：

```text
CREATE TABLE
PRIMARY KEY
UNIQUE
INSERT
SELECT
psql 基本操作
```

### 第三段：复盘表达

每天结束前，用自己的话回答 5 到 8 个问题。

例如 Day 02 可以问：

```text
为什么要有 config.go？
为什么 Docker PostgreSQL 使用 15432？
schema.sql 和真实数据库表有什么区别？
为什么不能保存明文密码？
handler 为什么需要 dbPool？
JWT 为什么需要 secret？
router.go 在请求流转里负责什么？
```

如果能回答这些，说明不是机械照抄。

## 项目知识点对接表

| 项目模块 | 你要掌握的基础 | 推荐资料 |
| --- | --- | --- |
| Go 基础 | 变量、函数、结构体、包、错误处理、接口、context | 你提到的 Go 教程：https://golang.halfiisland.com/guide.html |
| Go 官方基础 | Go 语言教程、模块、测试、数据库访问 | https://go.dev/doc/docs.html |
| Gin Web | 路由、JSON、请求绑定、中间件、错误返回 | https://gin-gonic.com/en/docs/ |
| PostgreSQL | 数据库、表、主键、唯一约束、SQL、psql | https://www.postgresql.org/docs/current/tutorial.html |
| Go 连接数据库 | 连接池、QueryRow、Scan、参数化 SQL | https://go.dev/doc/tutorial/database-access |
| pgx | PostgreSQL Go 驱动、连接池、查询 | https://pkg.go.dev/github.com/jackc/pgx/v5 |
| Docker Compose | 容器、镜像、端口映射、volume、服务编排 | https://docs.docker.com/compose/ |
| 密码安全 | bcrypt、密码哈希、不能存明文密码 | https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html |
| JWT | token、claims、过期时间、签名密钥 | https://pkg.go.dev/github.com/golang-jwt/jwt/v5 |
| Redis 基础 | String、Hash、Set、ZSet、TTL | https://redis.io/docs/latest/develop/data-types/ |
| Redis 排行榜 | Sorted Set、score、rank | https://redis.io/docs/latest/develop/data-types/sorted-sets/ |
| WebSocket | 长连接、消息发送、连接状态、断开处理 | https://developer.mozilla.org/en-US/docs/Web/API/WebSocket |
| Go 测试 | `go test`、单元测试、表驱动测试 | https://go.dev/doc/tutorial/add-a-test |
| React | 组件、状态、事件、表单、接口请求 | https://react.dev/learn |

## 按项目阶段学习

### 阶段 1：Go 后端基础骨架

对应 Day 01 到 Day 02 前半段。

需要掌握：

- Go module 是什么
- `go.mod` 和 `go.sum` 的作用
- `cmd/server/main.go` 为什么是入口
- `internal/router` 为什么管理路由
- `internal/handler` 为什么处理 HTTP 请求
- `/health` 为什么是第一个接口

你要能画出：

```text
浏览器 / Apifox
        ↓
GET /health
        ↓
router.go
        ↓
handler.Health
        ↓
JSON 响应
```

### 阶段 2：配置、数据库、注册登录

对应 Day 02。

需要掌握：

- 环境变量
- 配置默认值
- Docker Compose
- PostgreSQL 端口映射
- 数据库连接池
- SQL 建表
- bcrypt 密码哈希
- JWT 登录令牌

你要能画出注册调用链：

```text
Apifox
        ↓
POST /api/register
        ↓
router.go
        ↓
AuthHandler.Register
        ↓
bcrypt.GenerateFromPassword
        ↓
INSERT INTO players
        ↓
PostgreSQL
        ↓
返回 JSON
```

你要能画出登录调用链：

```text
Apifox
        ↓
POST /api/login
        ↓
AuthHandler.Login
        ↓
SELECT player by username
        ↓
bcrypt.CompareHashAndPassword
        ↓
GenerateToken
        ↓
返回 JWT
```

### 阶段 3：玩家模块和基础权限

对应第 2 周。

需要掌握：

- 鉴权中间件
- `Authorization: Bearer token`
- 从 JWT 中解析 player_id
- 查询当前玩家信息
- GM 管理员和普通玩家的区别
- 基础 RBAC 思想

这一阶段要理解：

```text
登录只是拿到 token
鉴权才是后续接口判断身份
权限控制才是判断这个身份能不能做某件事
```

### 阶段 4：WebSocket 长连接

对应第 3 周。

需要掌握：

- HTTP 和 WebSocket 的区别
- 为什么实时游戏需要长连接
- 连接建立、消息读取、消息发送、连接断开
- 心跳机制
- 在线状态
- 连接和玩家 ID 的映射关系

你要能画出：

```text
玩家登录拿到 JWT
        ↓
连接 /ws?token=xxx
        ↓
后端校验 token
        ↓
建立 WebSocket 连接
        ↓
记录 player_id -> connection
        ↓
收发实时消息
```

### 阶段 5：房间系统

对应第 4 周。

需要掌握：

- 房间数据结构
- 加入房间、退出房间
- 房间广播
- 房间状态存储
- 并发安全
- Redis 缓存房间状态

重点不是做复杂游戏规则，而是理解多人实时服务里的状态管理。

### 阶段 6：匹配队列、结算、排行榜

对应第 5 周。

需要掌握：

- Redis List / Set / ZSet 的区别
- 匹配队列如何入队和出队
- 如何防止重复结算
- 排行榜为什么适合用 ZSet
- 战绩为什么要持久化到 PostgreSQL

你要能讲清楚：

```text
Redis 负责实时和高频状态
PostgreSQL 负责长期可靠数据
```

### 阶段 7：React GM 后台

对应第 6 周。

需要掌握：

- React 组件
- 表单
- 表格
- 请求后端 API
- Token 存储和携带
- GM 后台常见页面结构

不要把它做成花哨官网。GM 后台应该是工具型界面，重点是查询、筛选、操作和监控。

### 阶段 8：测试、压测、pprof、文档

对应第 7 到第 8 周。

需要掌握：

- 单元测试
- 接口测试
- 压测指标
- pprof 观察性能
- 日志排查问题
- README 如何让别人跑起来
- 架构图如何表达系统结构
- 简历如何写项目亮点

这一阶段决定项目能不能从“能跑”变成“能讲、能演示、能面试”。

## 每个功能完成后的复盘模板

每完成一个功能，在文档或学习笔记里写下面这些内容。

```markdown
## 功能名称

### 这个功能解决什么问题

### 涉及哪些文件

### 请求或数据流怎么走

### 用到了哪些技术点

### 我遇到了什么错误

### 我是怎么排查的

### 如果面试官问，我怎么讲
```

示例：

```markdown
## 注册接口

### 这个功能解决什么问题

让新玩家创建账号，并把账号信息保存到数据库。

### 涉及哪些文件

- router.go：注册 POST /api/register 路由
- auth.go：处理注册请求
- player.go：描述玩家数据结构
- schema.sql：记录 players 表结构

### 请求或数据流怎么走

Apifox -> router.go -> AuthHandler.Register -> bcrypt -> PostgreSQL -> JSON 响应
```

## 每周复盘模板

每周结束时，写一次更大的复盘。

```markdown
# 第 X 周复盘

## 本周完成了什么

## 我新学会了什么

## 哪些地方还不熟

## 哪些错误反复出现

## 下周要重点补什么

## 这一周的内容怎么写进简历

## 这一周的内容面试可能怎么问
```

## 不同知识的学习深度

不是所有知识都要学到同样深度。

### 必须重点掌握

- Go 基础语法
- Go 项目结构
- Gin 路由和 handler
- PostgreSQL 基础 SQL
- Go 连接数据库
- Docker Compose 基础
- 注册、登录、JWT
- WebSocket 基本收发
- Redis 基础数据结构

这些是项目主线，必须能写、能跑、能解释。

### 需要理解并会使用

- bcrypt 密码哈希
- HTTP 状态码
- JSON 请求响应
- 日志
- 单元测试
- 接口测试
- 简单压测
- pprof 基础使用

这些不一定要一开始很深，但要知道解决什么问题。

### 后期再深入

- 微服务
- Kubernetes
- 服务发现
- 分布式锁
- 消息队列
- 高级监控告警
- 大规模游戏服务器架构

这些先不要急。它们是扩展方向，不是当前两个月项目的核心。

## 推荐学习顺序

### 当前阶段

先补这些：

1. Go 结构体、方法、包、错误处理
2. Gin 路由、请求绑定、JSON 响应
3. PostgreSQL 表、插入、查询、唯一约束
4. Docker Compose 端口映射、volume
5. bcrypt 和 JWT 基本概念

### WebSocket 前

再补这些：

1. HTTP 和 WebSocket 的区别
2. goroutine 和 channel 基础
3. map、mutex、并发安全
4. JSON 消息格式设计
5. 心跳和断线处理

### Redis 前

再补这些：

1. Redis String、Hash、Set、ZSet
2. TTL
3. 排行榜 Sorted Set
4. 在线状态 key 设计
5. 缓存和数据库的分工

### 前端 GM 后台前

再补这些：

1. React 组件
2. useState / useEffect
3. 表单和表格
4. fetch/axios 请求接口
5. Token 携带方式

## 学习时不要做的事

不要这样学：

- 连续看 Go 教程很多天，不写项目。
- 连续写项目，但不知道每行代码为什么存在。
- 一遇到报错就跳过，不记录原因。
- 一上来追求微服务、Kubernetes、复杂架构。
- 为了简历堆技术名词，却讲不清楚数据流。
- 只记命令，不理解命令在哪个目录执行、为什么执行。

正确目标是：

```text
小步实现
及时验证
回头理解
整理表达
```

## 面试表达训练

每个模块完成后，都练习用 1 分钟讲清楚。

例如 Day 02 可以这样讲：

```text
项目初期我先搭建了 Go + Gin 的后端骨架，并通过 Docker Compose 提供 PostgreSQL 和 Redis 依赖服务。随后抽离配置模块，使用环境变量管理服务端口、数据库连接信息和 JWT 密钥。数据库层使用 pgxpool 创建 PostgreSQL 连接池，并通过 Ping 确认连接可用。账号模块设计了 players 表，注册时使用 bcrypt 保存 password_hash，登录时校验密码并签发 JWT，为后续 WebSocket 玩家身份识别、房间系统和 GM 后台鉴权打基础。
```

这段话以后还可以继续优化成简历项目描述。

## 最重要的学习原则

这个项目不是为了把功能堆满，而是为了形成工程师思维。

工程师思维包括：

- 知道一个功能为什么要做
- 知道代码应该放在哪里
- 知道数据从哪里来、到哪里去
- 知道失败时怎么排查
- 知道如何验证功能正确
- 知道如何把实现讲清楚

只要你每个模块都按这个方式学，这个项目就不只是一个简历项目，而会变成你后端工程能力的骨架。

