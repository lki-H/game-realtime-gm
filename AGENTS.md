# game-realtime-gm 项目协作规则

本文件是 `game-realtime-gm` 项目的项目级 `AGENTS.md`，由原 `claude.md` 与项目 GitHub 流程规则整合而来。

以后本项目只维护这一份项目协作规则，不再单独维护 `claude.md`。

当本文件与用户全局 `C:\Users\lki\.codex\AGENTS.md` 不冲突时，本文件优先补充本项目的具体工作方式。

## 项目定位

这是一个 Go 游戏后台与 GM 运营后台学习项目。

当前目标不是一次性做成大型商业系统，而是通过每天一个小功能，逐步训练：

- Go 后端工程结构
- Gin HTTP API
- PostgreSQL 数据建模
- Redis 实时状态
- JWT 鉴权和权限隔离
- GM 后台业务闭环
- 操作日志审计
- Docker Compose 本地环境
- GitHub 持续提交和求职展示

面向 HR 或面试官时，不要把项目包装成已经商用的大型游戏服务器。应如实表达为：

```text
持续迭代中的 Go 游戏后台与 GM 运营后台学习项目。
```

## 使用者学习背景

使用者是刚接触 Go 后端、数据库、Redis、Docker、WebSocket、React GM 后台等技术的学生。

因此，后续文档不能只给任务清单，也不能只给命令和代码。每一步都需要解释清楚：

- 为什么要做这个操作
- 这个操作在真实后端项目中有什么作用
- 新增或修改的文件负责什么
- 这个文件以后可能会扩展成什么
- 怎么验证这一步真的成功了
- 如果失败，优先检查什么

文档应该像老师带学生做项目一样：

- 讲清楚，但不要故意讲复杂
- 可以类比，但不要脱离工程实际
- 不要只堆术语
- 不要默认使用者已经懂 Docker、数据库、JWT、Redis
- 每个阶段都要告诉使用者“你现在学会了什么”

## 每次生成每日任务文档前必须读取

当用户要求继续 `dayXX`、生成每日任务文档、规划下一天学习任务、补项目学习文档时，优先读取：

```text
E:\game-realtime-gm\README.md
E:\game-realtime-gm\docs\learning-roadmap.md
E:\game-realtime-gm\docs\project-technical-standards.md
E:\game-realtime-gm\docs\api-overview.md
E:\game-realtime-gm\docs\github-workflow.md
E:\game-realtime-gm\docs\day上一天-plan.md
```

如果上下文不安全、跨天太多、或旧对话压缩可能影响判断，再读取：

```text
E:\game-realtime-gm\docs\conversation-handoff-gpt55.md
E:\game-realtime-gm\docs\codex-context.md
```

注意：`conversation-handoff-gpt55.md` 和 `codex-context.md` 是内部交接资料，不建议上传到 GitHub。

## 每日任务文档创建要求

每次生成每日计划时，必须在项目文档目录创建或更新对应文件：

```text
E:\game-realtime-gm\docs\dayXX-plan.md
```

命名规则：

- `XX` 使用两位数字，例如 `day02-plan.md`、`day03-plan.md`、`day10-plan.md`。
- 文件放在项目根目录下的 `docs/` 目录。
- 如果当天计划文件已经存在，先读取已有内容，再在不丢失重要信息的前提下更新。
- 不能只在聊天窗口输出计划而不写入文件，除非用户明确说“只在聊天里说，不要创建文件”。

## 每日任务文档最低结构

每日任务文档必须适合初学者照着做，至少包含：

- 今天从任务几开始
- 今日目标
- 今日最终效果
- 今日会学到什么
- 今日文件范围
- 具体文件路径
- 搜索关键词
- 插入或替换位置
- 需要粘贴的代码
- gofmt 步骤
- `go test ./...` 验证步骤
- Apifox 或 curl 测试步骤
- PostgreSQL 或 Redis 对照验证步骤
- 常见错误和定位方式
- 今日验收清单
- 今日不要做什么
- 明日预告
- GitHub 收尾流程

建议结构：

```markdown
# Day XX 学习计划：今日主题

## 你今天从任务几开始

## 今日目标

## 今日最终效果

## 今日会学到什么

## 今日文件范围

## 任务 0：确认上一天状态

## 任务 1：任务名称

### 你要做什么

### 为什么要做

### 修改文件

### 修改位置

### 代码或配置

### 怎么验证成功

## 今日验收清单

## 常见问题

## 今日不要做什么

## 任务 N：提交并推送到 GitHub

## 明日预告
```

## 精细任务讲解强制要求

后续输出每日任务时，每一个任务都必须达到类似“任务 2：配置读取”那样的讲解颗粒度。

不能只写：

```text
创建配置文件，读取环境变量。
```

必须写清楚：

- 这个任务最终要实现什么能力
- 要新增或修改哪些文件
- 为什么要执行这些操作
- 每个文件在项目中负责什么
- 每一步命令在哪个目录执行
- 每一步命令执行后应该看到什么
- 每个代码文件具体要写入什么内容
- 如果修改已有文件，明确说明是“整文件替换”还是“只修改某几行”
- 如果只修改某几行，必须给出修改前和修改后的代码片段
- 代码或配置的完整内容
- 代码或配置逐段分析
- 这段代码和项目其他部分的关系
- 怎么验证成功
- 常见错误和排查方法

每个任务都必须同时包含两类解释：

1. 操作分析：解释为什么要执行这些命令、为什么要创建这些目录和文件。
2. 代码分析：解释代码或配置每一段在做什么，以及它在项目中的位置。

如果某个任务没有 Go 代码，也必须做配置分析或命令分析。

例如：

- Docker Compose 任务要解释 `services`、`image`、`ports`、`volumes` 的作用。
- SQL 建表任务要解释表名、字段、主键、唯一约束、时间字段的作用。
- 环境变量任务要解释每个变量保存什么信息、为什么不能写死在代码里。
- Git 任务要解释提交记录的意义、为什么某些文件不应该提交。

## 具体代码操作要求

后续每个涉及代码的任务，必须提供可以直接照着执行的代码操作，不能只说“实现某接口”“新增某模块”“修改路由”。

每个代码任务必须包含：

- 文件路径：完整写出要编辑的文件，例如 `E:\game-realtime-gm\backend\internal\handler\auth.go`。
- 操作类型：明确是“新建文件”“填写空文件”“整文件替换”“在某段代码后新增”“把 A 改成 B”。
- 完整代码：对于新建文件、空文件、短文件，必须给出完整文件内容。
- 修改片段：对于已有文件，如果不适合整文件替换，必须给出“修改前”和“修改后”。
- 执行命令：写完代码后要执行哪些命令，例如 `gofmt`、`go mod tidy`、`go test ./...`。
- 验证结果：命令成功后应该看到什么输出，接口成功后应该返回什么 JSON。

示例：新建或填写空文件时，必须这样写：

```text
文件位置：
E:\game-realtime-gm\backend\internal\auth\jwt.go

操作类型：
新建文件。如果文件已经存在但为空，直接填入下面完整内容。
```

示例：修改已有文件时，必须这样写：

```text
文件位置：
E:\game-realtime-gm\backend\internal\router\router.go

操作类型：
整文件替换。
```

示例：只改某一行时，必须这样写：

```go
// 修改前
r := router.New()

// 修改后
r := router.New(dbPool, cfg)
```

如果一个任务需要改多个文件，必须按文件分别列出，不允许把所有代码混在一起讲。

每个任务末尾必须有一个“完成后执行”小节，例如：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\auth\jwt.go .\internal\handler\auth.go
go mod tidy
go test ./...
```

并解释每条命令的作用。

## 解释新概念的要求

写计划和说明时，要优先使用学生能理解的语言。

遇到新概念时，先用一句话解释它的作用，再进入操作。例如：

- `go.mod`：Go 项目的依赖清单和模块身份证。
- `router.go`：集中登记接口地址和对应处理函数。
- `handler`：真正处理请求、返回 JSON 的地方。
- `docker-compose.yml`：一键启动项目依赖服务的说明书。
- PostgreSQL：保存长期业务数据，例如账号、玩家、战绩。
- Redis：保存高频、临时、实时状态，例如在线状态、匹配队列、排行榜。

涉及命令时，需要说明：

- 在哪个目录执行
- 命令的作用
- 成功后应该看到什么
- 如果失败，优先检查什么

涉及新增文件或目录时，需要说明它的职责。

例如：

```text
backend/internal/config/
```

说明：

- 用来集中读取和管理配置。
- 数据库地址、端口、账号密码、JWT 密钥这类会变化的信息不应该写死在业务代码里。
- 后续开发环境、测试环境、线上环境可以使用不同配置。

每次修改已有文件，也要说明为什么要改这个文件，而不是改其他文件。

## 代码与配置分析要求

当任务包含 Go 代码时，必须解释：

- `package` 表示这个文件属于哪个包
- `import` 引入了哪些能力
- 新增的 `struct` 表示什么数据
- 新增的函数负责什么
- 函数参数和返回值分别是什么
- 为什么这样组织代码
- 这段代码后续会被哪里调用

当任务包含 YAML、JSON、SQL 或环境变量时，也必须逐项解释关键字段。

`docker-compose.yml` 至少要解释：

- `services` 表示要启动哪些服务
- `image` 表示使用哪个 Docker 镜像
- `container_name` 表示容器名称
- `environment` 表示容器启动时的环境变量
- `ports` 表示本机端口和容器端口的映射
- `volumes` 表示数据持久化位置

SQL 表结构至少要解释：

- 表名表示什么业务对象
- 每个字段保存什么数据
- 哪些字段必须唯一
- 为什么密码字段保存 `password_hash` 而不是明文密码
- `created_at` 和 `updated_at` 的作用

接口任务至少要解释：

- 接口路径和 HTTP 方法的含义
- 请求 JSON 每个字段的作用
- 响应 JSON 每个字段的作用
- 正常情况返回什么
- 常见错误情况返回什么

## Go 与工程结构要求

后续实现代码时，应保持结构清晰，不为了炫技而复杂化。

建议遵循：

- 路由放在 `internal/router`
- HTTP 处理函数放在 `internal/handler`
- 配置读取放在 `internal/config`
- 数据库连接放在 `internal/database`
- Redis 连接放在 `internal/cache`
- 业务逻辑后续可放在 `internal/service`
- 数据结构和数据库模型放在 `internal/model`

如果项目已有既定结构，应优先沿用已有结构。

必须遵守：

- 同一目录且同一 `package` 的 Go 文件属于同一个包。
- 同一个包内不能生成重复函数或重复类型。
- helper 函数如果只被一个文件使用，放在当前文件底部。
- helper 函数如果被多个同包文件复用，应放到同包公共 helper 文件或更合适的包中。
- 每次生成涉及代码的计划时，要检查新增函数是否会和已有文件重名。

当前阶段不要提前引入：

- 微服务
- Kubernetes
- 服务网格
- 消息队列
- 分布式锁
- 复杂 CQRS

除非当前功能真的需要。

## 不要自动替用户乱改代码

用户曾明确要求：出现代码问题时，不要自行修改，先告诉用户怎么解决。

因此：

- 如果用户只是问错误原因，先解释问题和解决步骤。
- 如果用户要求生成每日任务文档，可以直接创建文档。
- 如果用户明确要求“帮我改”，再修改代码。
- 如果代码修改风险较高，先说明影响范围。

## 学习节奏要求

项目以两个月可演示、可写进简历为目标，但每天应控制范围。

原则：

- 先做能跑通的最小闭环，再逐步增强。
- 每天只聚焦一个主线，不同时开太多技术点。
- 每天结束时必须有可验证结果。
- 对初学者来说，理解“为什么这样设计”比堆功能更重要。

每天结束前，尽量帮助用户能回答：

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

## 验证优先级

后端任务默认至少验证：

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

涉及 Docker、数据库、Redis 时，根据任务补充：

```powershell
cd E:\game-realtime-gm\deploy
docker ps
```

数据库：

```powershell
docker exec -it game_realtime_postgres psql -U game -d game_realtime
```

Redis：

```powershell
docker exec -it game_realtime_redis redis-cli
```

如果无法验证，必须说明原因和剩余风险。

## GitHub 收尾流程必须写入每日任务文档

每个 `dayXX-plan.md` 末尾必须加入 GitHub 收尾任务，标题建议为：

```text
## 任务 N：提交并推送到 GitHub
```

默认流程：

```powershell
cd E:\game-realtime-gm
git status
```

确认没有敏感文件后，选择性添加当天修改：

```powershell
git add backend docs/dayXX-plan.md README.md docs/api-overview.md
```

如果当天没有修改某些文件，不要强行添加。不要无脑使用 `git add .`。

提交：

```powershell
git commit -m "Complete dayXX ..."
```

推送：

```powershell
git push
```

推送后检查：

```powershell
git status -sb
```

理想状态：

```text
## main...origin/main
```

完整流程见：

```text
E:\game-realtime-gm\docs\github-workflow.md
```

## GitHub 上传前安全检查

每次提交或推送前必须提醒检查：

不要上传：

```text
.env
.env.*
*.pem
*.key
*.dump
*.sql.gz
*.log
真实手机号、真实邮箱等个人隐私
真实线上数据库密码
真实 JWT_SECRET
Docker volume 数据
GoLand 临时文件
```

本项目默认不建议上传：

```text
docs/codex-context.md
docs/conversation-handoff-gpt55.md
docs/mcp-adoption-plan.md
docs/skill-adoption-plan.md
```

这些文件偏内部协作和对话交接，不适合作为 HR 第一眼看到的项目材料。

## 分支和提交规则

当前远程仓库：

```text
https://github.com/lki01/game-realtime-gm.git
```

默认主分支：

```text
main
```

当前学习阶段，日常任务可以直接提交到 `main`，但必须先验证。

后期如果开始做更大功能，可以使用功能分支：

```text
feature/day16-dashboard
feature/day17-tests
```

但用户仍处于 GitHub 学习早期时，不要强行引入复杂分支流程。

提交信息建议：

```text
Complete day16 dashboard stats
Update API overview after day16
Fix operation log detail query
```

每天建议 1 个主 commit，必要时 2 个 commit。

## 实际修改项目后的工作日志

实际修改项目文件后，必须维护每日工作日志：

```text
D:\桌面\codex_log\YYYY-MM-DD.md
```

日志包含：

- 请求摘要
- 澄清结果
- 修改范围
- touched files
- 关键决策
- 验证结果
- 剩余风险
- 下一步建议

如果当天执行了 GitHub 提交或推送，也要记录：

```text
git status
commit hash
push 结果
未提交文件
```

## 项目展示导向

面向 HR 或面试官时，优先展示：

- README
- docs/api-overview.md
- docs/dayXX-plan.md
- 清晰的 GitHub commit 记录
- 可运行的 Docker Compose 环境
- `go test ./...` 通过
- 业务闭环：登录、鉴权、玩家管理、封禁解封、GM 操作日志

如果时间允许，每天文档可以额外补充：

- 面试怎么讲这一部分
- 简历中怎么描述这一部分
- 后续会如何扩展
