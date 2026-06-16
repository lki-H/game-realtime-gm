# Day 09 学习计划：GM 操作日志基础

## 你今天从任务几开始

你现在从：

```text
任务 0：先确认 Day 08 已经能跑
```

开始。

因为 Day 09 依赖 Day 08 的两个管理员接口：

```text
GET /api/admin/players
GET /api/admin/players/:id
```

如果 Day 08 还没跑通，Day 09 的操作日志就没有地方记录。

我根据当前项目文件判断：你现在的代码里已经有 Day 08 的实现，所以正常情况下任务 0 只需要快速验证，然后进入任务 1。

## 今日目标

今天做 GM 后台的一个基础能力：

```text
GM 操作日志
```

一句话理解：

```text
管理员登录后访问 GM 接口时，系统要记录“谁在什么时候做了什么”。
```

今天先记录两个查询操作：

```text
GET /api/admin/players
GET /api/admin/players/:id
```

也就是：

```text
管理员查询玩家列表
管理员查询玩家详情
```

今天不做封禁玩家、不做解封、不做前端日志页面。

## 为什么 Day 09 做操作日志

Day 06 到 Day 08 已经完成了这条线：

```text
管理员账号
  ↓
管理员登录
  ↓
管理员 token
  ↓
管理员鉴权
  ↓
GM 查询玩家
```

一旦 GM 后台开始能看玩家、改玩家，系统就必须留下记录。

真实后台里，操作日志很重要，原因有三个：

- 出问题时能追查是谁操作的。
- 管理员误操作时能定位时间和目标对象。
- 后续做封禁、改昵称、补偿道具时，必须有审计依据。

今天先从风险较低的查询日志开始。

## 今日最终效果

完成后，你用管理员 token 请求：

```text
GET http://localhost:8080/api/admin/players?page=1&page_size=10
```

接口仍然正常返回玩家列表。

同时 PostgreSQL 里的：

```text
admin_operation_logs
```

会多一条记录，类似：

```text
admin_id       = 1
admin_username = admin
admin_role     = super_admin
action         = admin.players.list
target_type    = player
target_id      = NULL
detail         = page=1,page_size=10
```

你请求：

```text
GET http://localhost:8080/api/admin/players/1
```

日志表会多一条：

```text
action      = admin.players.detail
target_type = player
target_id   = 1
```

## 今天会学到什么

今天会学到：

- 什么是后台操作日志。
- 为什么 GM 后台一定要有审计记录。
- 如何设计操作日志表。
- 如何在 Docker Desktop 里的 PostgreSQL 执行建表 SQL。
- 如何把表结构补进 `schema.sql`。
- 如何在 Gin handler 里写一条操作日志。
- 为什么日志里不能记录密码、token 等敏感信息。

## 任务 0：先确认 Day 08 状态

### 任务目标

确认 Day 08 的管理员玩家查询接口能编译。

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

### 成功结果

看到类似：

```text
?   	game-realtime-gm/backend/cmd/server	[no test files]
?   	game-realtime-gm/backend/internal/config	[no test files]
?   	game-realtime-gm/backend/internal/handler	[no test files]
```

`[no test files]` 不是错误，只表示当前包还没有测试文件。

### 如果失败怎么办

如果这里失败，先不要继续 Day 09。

把完整报错发给我，我先告诉你原因和解决方法，再决定要不要改。

## 任务 1：理解今天要改哪些地方

今天涉及三个文件。

第一个是数据库表结构文件：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
```

这里要补上：

```text
admin_operation_logs 表结构
```

第二个是模型文件：

```text
E:\game-realtime-gm\backend\internal\model\admin_operation_log.go
```

这里定义 Go 里的操作日志结构体。

第三个是管理员业务 handler：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

这里要新增一个记录操作日志的辅助函数，并在两个管理员查询接口里调用它。

今天不需要改：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

原因是今天不新增 API，只是在现有 API 内部多做一件事：

```text
写操作日志
```

## 任务 2：在 Docker Desktop 里创建操作日志表

### 任务目标

先让 PostgreSQL 里真实存在：

```text
admin_operation_logs
```

### 在哪里操作

Docker Desktop。

### 操作步骤

打开 Docker Desktop。

进入当前项目的 PostgreSQL 容器。

容器名字可能类似：

```text
game-realtime-gm-postgres-1
game_realtime_postgres
postgres
```

进入容器的 Terminal 或 Exec 页面。

执行：

```bash
psql -U game -d game_realtime
```

成功后会看到：

```text
game_realtime=#
```

如果你已经在这个提示符里，就不用再执行 `psql`，直接粘贴下面的 SQL。

### 执行 SQL

在：

```text
game_realtime=#
```

里粘贴：

```sql
CREATE TABLE IF NOT EXISTS admin_operation_logs (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL,
    admin_username VARCHAR(64) NOT NULL,
    admin_role VARCHAR(32) NOT NULL,
    action VARCHAR(64) NOT NULL,
    target_type VARCHAR(64) NOT NULL,
    target_id BIGINT,
    detail TEXT NOT NULL DEFAULT '',
    ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_admin_id
ON admin_operation_logs (admin_id);

CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_action
ON admin_operation_logs (action);

CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_created_at
ON admin_operation_logs (created_at DESC);
```

### 成功结果

第一次执行通常看到：

```text
CREATE TABLE
CREATE INDEX
CREATE INDEX
CREATE INDEX
```

如果之前已经建过，可能看到：

```text
NOTICE:  relation "admin_operation_logs" already exists, skipping
CREATE TABLE
NOTICE:  relation "idx_admin_operation_logs_admin_id" already exists, skipping
CREATE INDEX
```

这也算成功。

### 验证表是否存在

继续在：

```text
game_realtime=#
```

里执行：

```sql
\d admin_operation_logs
```

应该能看到字段：

```text
id
admin_id
admin_username
admin_role
action
target_type
target_id
detail
ip
user_agent
created_at
```

### 这个表每个字段是什么意思

`id`：

```text
日志自己的主键 ID。
```

`admin_id`：

```text
执行操作的管理员 ID。
```

`admin_username`：

```text
执行操作的管理员用户名。
```

为什么已经有 `admin_id` 还要存 `admin_username`？

因为日志是审计记录，要尽量保留当时的信息快照。

如果以后管理员改名，老日志仍然能看出当时是谁操作的。

`admin_role`：

```text
执行操作时管理员的角色。
```

例如：

```text
super_admin
gm
```

`action`：

```text
具体做了什么动作。
```

今天用两个 action：

```text
admin.players.list
admin.players.detail
```

`target_type`：

```text
操作对象类型。
```

今天操作的是玩家，所以是：

```text
player
```

`target_id`：

```text
操作对象 ID。
```

查询玩家详情时，这里是玩家 ID。

查询玩家列表时，不是针对某一个玩家，所以这里是：

```text
NULL
```

`detail`：

```text
补充说明。
```

例如：

```text
page=1,page_size=10,keyword=test
```

`ip`：

```text
请求来源 IP。
```

本地开发时通常是：

```text
127.0.0.1
::1
```

`user_agent`：

```text
请求客户端信息。
```

Apifox、浏览器、脚本请求都会带不同的 user agent。

`created_at`：

```text
日志创建时间。
```

## 任务 3：把表结构补进 schema.sql

### 任务目标

刚才是在数据库里执行 SQL，只改变了 Docker 里的 PostgreSQL。

现在还要把同样的表结构写进项目文件：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
```

原因是：

```text
数据库里执行 SQL 是让当前环境生效。
schema.sql 是让项目代码保存这份表结构。
```

如果以后换电脑、删容器、重新初始化数据库，就要靠 `schema.sql` 重新建表。

### 怎么补

打开：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
```

在 `admins` 表后面追加：

```sql

CREATE TABLE IF NOT EXISTS admin_operation_logs (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL,
    admin_username VARCHAR(64) NOT NULL,
    admin_role VARCHAR(32) NOT NULL,
    action VARCHAR(64) NOT NULL,
    target_type VARCHAR(64) NOT NULL,
    target_id BIGINT,
    detail TEXT NOT NULL DEFAULT '',
    ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_admin_id
ON admin_operation_logs (admin_id);

CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_action
ON admin_operation_logs (action);

CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_created_at
ON admin_operation_logs (created_at DESC);
```

### 注意

`schema.sql` 里可以有多个：

```sql
CREATE TABLE IF NOT EXISTS ...
```

`IF NOT EXISTS` 的意思是：

```text
如果表不存在就创建；如果表已经存在就跳过。
```

所以这个 SQL 可以重复执行，不会因为表已经存在而失败。

## 任务 4：新增操作日志模型

### 任务目标

新增文件：

```text
E:\game-realtime-gm\backend\internal\model\admin_operation_log.go
```

### 为什么要有 model

数据库表是 PostgreSQL 里的结构。

Go 代码里也需要一个对应的结构体，方便后续返回日志列表、做接口文档、保持项目结构清晰。

今天这个结构体暂时主要是为后续 Day 10 或 Day 11 准备。

### 文件内容

创建文件：

```text
E:\game-realtime-gm\backend\internal\model\admin_operation_log.go
```

写入：

```go
package model

import "time"

type AdminOperationLog struct {
	ID            int64      `json:"id"`
	AdminID       int64      `json:"admin_id"`
	AdminUsername string     `json:"admin_username"`
	AdminRole     string     `json:"admin_role"`
	Action        string     `json:"action"`
	TargetType    string     `json:"target_type"`
	TargetID      *int64     `json:"target_id,omitempty"`
	Detail        string     `json:"detail"`
	IP            string     `json:"ip"`
	UserAgent     string     `json:"user_agent"`
	CreatedAt     time.Time  `json:"created_at"`
}
```

### 字段解释

这个结构体和数据库表基本一一对应。

`TargetID` 是：

```go
*int64
```

不是：

```go
int64
```

原因是 `target_id` 允许为空。

查询玩家列表时没有具体某一个玩家 ID，所以数据库里是 `NULL`。

Go 里用指针表示：

```text
有值或没有值
```

### 可能出现的格式问题

如果你复制后格式看起来不整齐，执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\model\admin_operation_log.go
```

## 任务 5：在 admin.go 里新增记录日志函数

### 任务目标

修改：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

给 `AdminHandler` 增加一个辅助方法：

```text
recordOperation
```

它负责往 `admin_operation_logs` 表写一行记录。

### 为什么不新建 service

今天只是学习操作日志基础。

当前项目还处于单体 Gin 服务学习阶段，逻辑也不复杂，所以先把日志辅助函数放在：

```text
handler/admin.go
```

等后续操作日志被多个 handler 使用，再考虑抽到：

```text
internal/service
internal/repository
```

今天不要提前引入复杂分层。

### 需要改 import

在 `admin.go` 的 import 里新增：

```go
"log"
```

也就是从：

```go
import (
	"errors"
	"net/http"
	"strconv"
	"strings"
```

改成：

```go
import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
```

### 新增函数放在哪里

建议放在：

```go
func (h *AdminHandler) Me(c *gin.Context) {
```

后面。

也就是：

```text
Me 方法结束之后
ListPlayers 方法之前
```

### 新增代码

```go
func (h *AdminHandler) recordOperation(c *gin.Context, action string, targetType string, targetID *int64, detail string) {
	adminID, ok := middleware.CurrentAdminID(c)
	if !ok {
		return
	}

	var targetValue any
	if targetID != nil {
		targetValue = *targetID
	}

	_, err := h.db.Exec(
		c.Request.Context(),
		`INSERT INTO admin_operation_logs (
			admin_id,
			admin_username,
			admin_role,
			action,
			target_type,
			target_id,
			detail,
			ip,
			user_agent
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		adminID,
		middleware.CurrentAdminUsername(c),
		middleware.CurrentAdminRole(c),
		action,
		targetType,
		targetValue,
		detail,
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	if err != nil {
		log.Printf("record admin operation failed: action=%s target_type=%s target_id=%v err=%v", action, targetType, targetValue, err)
	}
}
```

### 代码解释

```go
adminID, ok := middleware.CurrentAdminID(c)
```

从 Gin context 里取当前管理员 ID。

这个值来自：

```text
middleware.AdminAuth
```

也就是管理员 token 解出来的信息。

```go
if !ok {
	return
}
```

如果拿不到管理员身份，就不写日志。

正常情况下，能进 GM 接口就应该拿得到。

```go
var targetValue any
if targetID != nil {
	targetValue = *targetID
}
```

这里是为了处理：

```text
target_id 可以为空
```

查询玩家列表时传入 `nil`，数据库里就是 `NULL`。

查询玩家详情时传入玩家 ID，数据库里就是具体数字。

```go
h.db.Exec(...)
```

执行插入 SQL。

这里使用 `$1`、`$2` 这种参数占位符，不把用户输入直接拼进 SQL。

这是为了避免 SQL 注入风险。

```go
log.Printf(...)
```

如果写日志失败，先在控制台打印错误。

今天记录的是查询日志，为了不影响 GM 查询体验，日志失败暂时不让接口失败。

等后续做封禁、解封这类关键操作时，可以再讨论：

```text
关键操作日志写失败时，是否应该阻止操作成功。
```

### 安全注意

操作日志里不要记录这些内容：

```text
密码
password_hash
JWT token
Authorization header
数据库连接字符串
Redis 密码
```

今天的日志只记录：

```text
管理员身份
动作
目标对象
分页参数
IP
User-Agent
```

这些适合做审计。

## 任务 6：在查询玩家列表时记录日志

### 任务目标

在：

```text
ListPlayers
```

成功查询完玩家列表后，写一条操作日志。

### 修改位置

找到 `ListPlayers` 方法末尾：

```go
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"items":     players,
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
```

在这段 `c.JSON` 前面加入：

```go
	detail := "page=" + strconv.Itoa(page) + ",page_size=" + strconv.Itoa(pageSize)
	if keyword != "" {
		detail += ",keyword=" + keyword
	}
	h.recordOperation(c, "admin.players.list", "player", nil, detail)
```

### 修改后应该长这样

```go
	detail := "page=" + strconv.Itoa(page) + ",page_size=" + strconv.Itoa(pageSize)
	if keyword != "" {
		detail += ",keyword=" + keyword
	}
	h.recordOperation(c, "admin.players.list", "player", nil, detail)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"items":     players,
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
```

### 为什么放在成功查询之后

因为今天只记录：

```text
成功操作日志
```

如果 SQL 查询失败，接口返回错误，今天暂时不记录失败日志。

失败日志以后可以作为增强项。

## 任务 7：在查询玩家详情时记录日志

### 任务目标

在：

```text
GetPlayerByID
```

成功查到玩家详情后，写一条操作日志。

### 修改位置

找到 `GetPlayerByID` 方法末尾：

```go
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    player,
	})
```

在这段 `c.JSON` 前面加入：

```go
	h.recordOperation(c, "admin.players.detail", "player", &playerID, "query player detail")
```

### 修改后应该长这样

```go
	h.recordOperation(c, "admin.players.detail", "player", &playerID, "query player detail")

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    player,
	})
```

### 为什么 targetID 传 `&playerID`

`recordOperation` 的参数是：

```go
targetID *int64
```

也就是它要的是：

```text
int64 指针
```

`playerID` 本身是数字。

`&playerID` 表示：

```text
把 playerID 的地址传进去
```

这样函数就能判断：

```text
这次操作有明确目标 ID
```

并把它写入数据库的 `target_id` 字段。

## 任务 8：如果你怕放错位置，可以整文件替换 admin.go

### 任务目标

如果你不确定任务 5、6、7 的代码应该插在哪里，可以直接把：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

整文件替换成下面这一版。

### 完整 admin.go

```go
package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AdminHandler struct {
	db *pgxpool.Pool
}

func NewAdminHandler(db *pgxpool.Pool) *AdminHandler {
	return &AdminHandler{db: db}
}

func (h *AdminHandler) Me(c *gin.Context) {
	adminID, ok := middleware.CurrentAdminID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40115,
			"message": "admin identity missing",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"id":       adminID,
			"username": middleware.CurrentAdminUsername(c),
			"role":     middleware.CurrentAdminRole(c),
		},
	})
}

func (h *AdminHandler) recordOperation(c *gin.Context, action string, targetType string, targetID *int64, detail string) {
	adminID, ok := middleware.CurrentAdminID(c)
	if !ok {
		return
	}

	var targetValue any
	if targetID != nil {
		targetValue = *targetID
	}

	_, err := h.db.Exec(
		c.Request.Context(),
		`INSERT INTO admin_operation_logs (
			admin_id,
			admin_username,
			admin_role,
			action,
			target_type,
			target_id,
			detail,
			ip,
			user_agent
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		adminID,
		middleware.CurrentAdminUsername(c),
		middleware.CurrentAdminRole(c),
		action,
		targetType,
		targetValue,
		detail,
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	if err != nil {
		log.Printf("record admin operation failed: action=%s target_type=%s target_id=%v err=%v", action, targetType, targetValue, err)
	}
}

func (h *AdminHandler) ListPlayers(c *gin.Context) {
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "10"), 10)
	if pageSize > 50 {
		pageSize = 50
	}

	keyword := strings.TrimSpace(c.Query("keyword"))
	offset := (page - 1) * pageSize

	whereSQL := ""
	args := []any{}
	if keyword != "" {
		whereSQL = "WHERE username ILIKE $1 OR nickname ILIKE $1"
		args = append(args, "%"+keyword+"%")
	}

	var total int64
	countSQL := `SELECT COUNT(*) FROM players ` + whereSQL
	if err := h.db.QueryRow(c.Request.Context(), countSQL, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50041,
			"message": "count players failed",
		})
		return
	}

	listArgs := append(args, int32(pageSize), int32(offset))
	limitIndex := len(args) + 1
	offsetIndex := len(args) + 2

	rows, err := h.db.Query(
		c.Request.Context(),
		`SELECT id, username, nickname, created_at, updated_at
         FROM players
         `+whereSQL+`
         ORDER BY id DESC
         LIMIT $`+strconv.Itoa(limitIndex)+` OFFSET $`+strconv.Itoa(offsetIndex),
		listArgs...,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50042,
			"message": "query player list failed",
		})
		return
	}
	defer rows.Close()

	players := make([]model.Player, 0)
	for rows.Next() {
		var player model.Player
		if err := rows.Scan(
			&player.ID,
			&player.Username,
			&player.Nickname,
			&player.CreatedAt,
			&player.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50043,
				"message": "scan player failed",
			})
			return
		}
		players = append(players, player)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50044,
			"message": "read player rows failed",
		})
		return
	}

	detail := "page=" + strconv.Itoa(page) + ",page_size=" + strconv.Itoa(pageSize)
	if keyword != "" {
		detail += ",keyword=" + keyword
	}
	h.recordOperation(c, "admin.players.list", "player", nil, detail)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"items":     players,
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

func (h *AdminHandler) GetPlayerByID(c *gin.Context) {
	playerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || playerID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40041,
			"message": "invalid player id",
		})
		return
	}

	var player model.Player
	err = h.db.QueryRow(
		c.Request.Context(),
		`SELECT id, username, nickname, created_at, updated_at
         FROM players
         WHERE id = $1`,
		playerID,
	).Scan(
		&player.ID,
		&player.Username,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40441,
			"message": "player not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50045,
			"message": "query player failed",
		})
		return
	}

	h.recordOperation(c, "admin.players.detail", "player", &playerID, "query player detail")

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    player,
	})
}
```

### 特别注意

文件第一行必须是：

```go
package handler
```

`import (...)` 必须紧跟在 `package handler` 后面。

不要在 `package handler` 前面粘贴任何字符、注释、Markdown 标题或空的中文说明。

否则会再次出现类似：

```text
syntax error: non-declaration statement outside function body
imports must appear before other declarations
```

## 任务 9：格式化并编译

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\admin.go .\internal\model\admin_operation_log.go
go test ./...
```

### 成功结果

看到所有包通过。

类似：

```text
?   	game-realtime-gm/backend/cmd/server	[no test files]
?   	game-realtime-gm/backend/internal/handler	[no test files]
?   	game-realtime-gm/backend/internal/model	[no test files]
```

### 如果出现 `relation "admin_operation_logs" does not exist`

这不是编译错误，通常是在运行接口时出现。

原因是你还没有在 Docker Desktop 的 PostgreSQL 里执行任务 2 的建表 SQL。

解决：

回到任务 2，在：

```text
game_realtime=#
```

里执行建表 SQL。

### 如果出现 `undefined: middleware.CurrentAdminRole`

原因是你的 `middleware` 里可能还没有 `CurrentAdminRole` 这个函数。

先不要乱改。

你打开：

```text
E:\game-realtime-gm\backend\internal\middleware\admin_auth.go
```

看里面是否有：

```go
func CurrentAdminRole(c *gin.Context) string
```

如果没有，把报错和这个文件内容发给我，我会告诉你怎么补。

## 任务 10：启动后端

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

成功后应该看到：

```text
database connected
redis connected
server listening on :8080
```

### 如果 8080 被占用

如果看到：

```text
listen tcp :8080: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted.
```

说明已经有一个后端进程在占用 8080。

解决方法：

先找之前启动后端的 PowerShell 或 GoLand 运行窗口，按：

```text
Ctrl + C
```

停掉旧服务。

然后重新执行：

```powershell
go run .\cmd\server
```

如果你确定旧服务就是当前最新代码，也可以先直接测试接口。

但如果你刚刚改了代码，必须重启服务，新代码才会生效。

## 任务 11：用 Apifox 触发玩家列表日志

### 第 1 步：登录管理员

Apifox 请求：

```text
POST http://localhost:8080/api/admin/login
```

Body：

```json
{
  "username": "admin",
  "password": "admin123456"
}
```

复制响应里的：

```text
data.token
```

### 第 2 步：请求玩家列表

Apifox 请求：

```text
GET http://localhost:8080/api/admin/players?page=1&page_size=10
```

Headers：

```text
Authorization: Bearer 管理员token
```

成功响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "page": 1,
    "page_size": 10,
    "total": 0
  }
}
```

如果数据库里有玩家，`items` 会有数据。

### 第 3 步：到 Docker Desktop 里查日志

进入 PostgreSQL：

```bash
psql -U game -d game_realtime
```

执行：

```sql
SELECT id, admin_id, admin_username, admin_role, action, target_type, target_id, detail, ip, created_at
FROM admin_operation_logs
ORDER BY id DESC
LIMIT 10;
```

应该看到一条：

```text
action = admin.players.list
target_type = player
target_id = NULL
detail = page=1,page_size=10
```

## 任务 12：用 Apifox 触发玩家详情日志

### 第 1 步：先拿一个玩家 ID

请求：

```text
GET http://localhost:8080/api/admin/players?page=1&page_size=10
```

从响应里找到：

```text
data.items[0].id
```

如果没有玩家，先注册一个普通玩家。

普通玩家注册接口：

```text
POST http://localhost:8080/api/register
```

Body 示例：

```json
{
  "username": "testplayer",
  "password": "123456",
  "nickname": "测试玩家"
}
```

### 第 2 步：请求玩家详情

假设玩家 ID 是 `1`：

```text
GET http://localhost:8080/api/admin/players/1
```

Headers：

```text
Authorization: Bearer 管理员token
```

成功响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "username": "testplayer",
    "nickname": "测试玩家",
    "created_at": "...",
    "updated_at": "..."
  }
}
```

### 第 3 步：查询日志

Docker Desktop 的 PostgreSQL 里执行：

```sql
SELECT id, admin_username, action, target_type, target_id, detail, created_at
FROM admin_operation_logs
ORDER BY id DESC
LIMIT 10;
```

应该看到：

```text
action = admin.players.detail
target_type = player
target_id = 1
detail = query player detail
```

## 任务 13：测试权限隔离和日志行为

### 不带 token 请求

请求：

```text
GET http://localhost:8080/api/admin/players
```

不带 Authorization。

预期返回：

```json
{
  "code": 40112,
  "message": "authorization header missing"
}
```

日志表不应该新增成功操作日志。

原因：

```text
请求没有通过 AdminAuth，中间件直接拦截，根本不会进入 ListPlayers。
```

### 用玩家 token 请求

先登录普通玩家：

```text
POST http://localhost:8080/api/login
```

Body：

```json
{
  "username": "testplayer",
  "password": "123456"
}
```

复制玩家 token。

请求：

```text
GET http://localhost:8080/api/admin/players
```

Headers：

```text
Authorization: Bearer 玩家token
```

预期返回：

```json
{
  "code": 40311,
  "message": "admin permission required"
}
```

日志表也不应该新增成功操作日志。

原因同样是：

```text
请求没有通过 AdminAuth。
```

### 用管理员 token 请求

请求：

```text
GET http://localhost:8080/api/admin/players
```

Headers：

```text
Authorization: Bearer 管理员token
```

预期：

```text
接口成功，并新增一条操作日志。
```

## 常见问题

### 1. 接口成功了，但是日志表没有新记录

优先检查三件事。

第一，后端是不是重启过。

如果你改完代码没有重启，运行的还是旧代码。

第二，操作日志表是不是建在正确数据库里。

确认你进的是：

```bash
psql -U game -d game_realtime
```

第三，控制台有没有打印：

```text
record admin operation failed
```

如果有，把完整错误发给我。

### 2. 接口返回 500，并提示日志表不存在

原因：

```text
admin_operation_logs 表还没在 PostgreSQL 里创建。
```

解决：

执行任务 2 的 SQL。

### 3. 编译报 imported and not used

原因：

你 import 了某个包，但代码里没用到。

解决：

执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\admin.go
go test ./...
```

如果还报错，把完整错误发给我。

### 4. 日志里 target_id 是空

如果你请求的是：

```text
GET /api/admin/players
```

这是正常的。

因为玩家列表不是针对某一个玩家。

如果你请求的是：

```text
GET /api/admin/players/1
```

那么 `target_id` 应该是：

```text
1
```

### 5. User-Agent 很长

这是正常的。

User-Agent 本来就是客户端传来的字符串。

今天只是保存它，后续做日志列表接口时可以决定是否展示完整内容。

### 6. 要不要给 admin_id 加外键

今天先不加。

原因是操作日志属于审计记录，通常希望它保存当时的快照。

如果以后管理员账号被删除，日志仍然应该保留。

后续如果你想强约束，也可以加：

```sql
FOREIGN KEY (admin_id) REFERENCES admins(id)
```

但学习阶段先保持简单。

## 今日验收清单

- [ ] 在 Docker Desktop 的 PostgreSQL 里创建了 `admin_operation_logs` 表。
- [ ] `schema.sql` 已经补上 `admin_operation_logs` 表结构。
- [ ] 新增了 `internal/model/admin_operation_log.go`。
- [ ] `admin.go` 新增了 `recordOperation` 方法。
- [ ] `ListPlayers` 成功后会记录 `admin.players.list`。
- [ ] `GetPlayerByID` 成功后会记录 `admin.players.detail`。
- [ ] `go test ./...` 通过。
- [ ] 管理员 token 请求玩家列表后，日志表新增记录。
- [ ] 管理员 token 请求玩家详情后，日志表新增记录。
- [ ] 不带 token 请求 GM 接口不会新增成功日志。
- [ ] 玩家 token 请求 GM 接口不会新增成功日志。
- [ ] 日志中没有记录密码、password_hash、JWT token。

## 今日不要做

今天不要做：

- 封禁玩家。
- 解封玩家。
- 删除玩家。
- 修改玩家昵称。
- 给 GM 做多角色权限表。
- 做操作日志列表 API。
- 做 React 后台页面。
- 做 WebSocket。
- 做房间系统。

原因：

今天只完成一个基础闭环：

```text
管理员访问 GM 接口
  ↓
接口成功
  ↓
写入 GM 操作日志
  ↓
在 PostgreSQL 中查到审计记录
```

这个闭环跑通后，再做封禁玩家会更稳。

## 请求链路复盘

### 管理员查询玩家列表并写日志

```text
Apifox
  ↓
GET /api/admin/players
  ↓
router.go
  ↓
middleware.AdminAuth
  ↓
AdminHandler.ListPlayers
  ↓
SELECT COUNT(*) FROM players
  ↓
SELECT ... FROM players LIMIT/OFFSET
  ↓
INSERT INTO admin_operation_logs
  ↓
JSON response
```

### 管理员查询玩家详情并写日志

```text
Apifox
  ↓
GET /api/admin/players/:id
  ↓
router.go
  ↓
middleware.AdminAuth
  ↓
AdminHandler.GetPlayerByID
  ↓
SELECT ... FROM players WHERE id = $1
  ↓
INSERT INTO admin_operation_logs
  ↓
JSON response
```

## 面试怎么讲这一部分

可以这样讲：

```text
在 GM 后台查询玩家功能之后，我补充了管理员操作日志。日志表记录 admin_id、admin_username、admin_role、action、target_type、target_id、detail、IP、User-Agent 和创建时间。管理员访问 /api/admin/players 或 /api/admin/players/:id 成功后，系统会写入 admin_operation_logs，用于审计“谁在什么时候对什么对象做了什么”。实现时使用 JWT 管理员鉴权中间件提供的管理员身份，并使用 PostgreSQL 参数化 SQL 写入日志，避免把用户输入直接拼接进 SQL。日志中不会记录密码哈希、JWT token 等敏感信息。
```

## 明日预告

Day 10 建议做：

```text
GM 封禁玩家基础
```

原因是：

```text
Day 08 已经能查询玩家。
Day 09 已经能记录 GM 操作日志。
Day 10 就可以开始做第一个真正改变玩家状态的 GM 操作。
```

封禁玩家时，操作日志就会变得更有意义。
