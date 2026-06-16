# Day 10 学习计划：GM 封禁玩家基础

## 你今天从任务几开始

你现在从：

```text
任务 0：确认 Day 09 操作日志已经跑通
```

开始。

你已经在 PostgreSQL 里查到了：

```text
action = admin.players.list
```

这说明 Day 09 的“GM 查询玩家并写日志”核心链路已经成功。

Day 10 要在这个基础上做第一个真正改变玩家状态的 GM 操作：

```text
封禁玩家
```

## 今日目标

今天做一个最小可用的 GM 封禁闭环：

```text
管理员封禁玩家
  ↓
players 表记录封禁状态
  ↓
admin_operation_logs 记录封禁操作
  ↓
被封玩家再次登录时被拒绝
```

今天新增一个 GM 接口：

```text
POST /api/admin/players/:id/ban
```

请求 body：

```json
{
  "reason": "测试封禁"
}
```

成功后，玩家表里的状态会变成：

```text
status = banned
```

同时操作日志表会新增：

```text
action = admin.players.ban
target_type = player
target_id = 被封玩家ID
detail = reason=测试封禁
```

## 今天不做什么

今天不做：

- 解封玩家。
- 封禁时长。
- 自动过期解封。
- 多级 GM 权限。
- 操作日志查询接口。
- React GM 后台页面。
- Redis token 黑名单。
- 已登录玩家 token 立即失效。

原因是今天先完成最小闭环。

特别说明：

```text
Day 10 先阻止被封玩家“再次登录”。
如果玩家在封禁前已经拿到了 token，这个旧 token 是否立即失效，后面再做。
```

要让旧 token 立即失效，需要让玩家鉴权中间件每次查数据库状态，或做 Redis token 版本/黑名单。这会把今天内容拉得太大。

## 为什么今天做封禁玩家

GM 后台的核心能力不只是查询，还要处理异常玩家。

游戏服务端常见的 GM 操作有：

- 封禁玩家。
- 解封玩家。
- 改昵称。
- 修改玩家资产。
- 发补偿。
- 踢下线。

其中最适合现在做的是：

```text
封禁玩家
```

因为它能串起这些知识：

- 数据库字段扩展。
- 管理员权限接口。
- 状态变更 SQL。
- 事务。
- 操作日志。
- 登录拦截。
- Apifox 测试。
- Docker Desktop 查数据库。

## 今日最终效果

管理员调用：

```text
POST http://localhost:8080/api/admin/players/1/ban
```

Headers：

```text
Authorization: Bearer 管理员token
```

Body：

```json
{
  "reason": "测试封禁"
}
```

成功响应类似：

```json
{
  "code": 0,
  "message": "ban player success",
  "data": {
    "id": 1,
    "username": "testplayer",
    "nickname": "测试玩家",
    "status": "banned",
    "banned_reason": "测试封禁"
  }
}
```

然后普通玩家再登录：

```text
POST http://localhost:8080/api/login
```

预期返回：

```json
{
  "code": 40321,
  "message": "player is banned"
}
```

## 今日文件范围

今天建议修改这些文件：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
E:\game-realtime-gm\backend\internal\model\player.go
E:\game-realtime-gm\backend\internal\handler\admin.go
E:\game-realtime-gm\backend\internal\handler\auth.go
E:\game-realtime-gm\backend\internal\handler\player.go
E:\game-realtime-gm\backend\internal\router\router.go
```

每个文件的作用：

`schema.sql`：

```text
保存 players 表新增的封禁字段。
```

`model/player.go`：

```text
让 Go 结构体知道玩家状态、封禁原因、封禁时间、封禁管理员。
```

`handler/admin.go`：

```text
新增 GM 封禁玩家接口。
```

`handler/auth.go`：

```text
玩家登录时检查 status，如果是 banned 就拒绝登录。
```

`handler/player.go`：

```text
查询玩家时把 status 等字段一起查出来，避免响应里状态为空。
```

`router.go`：

```text
注册 POST /api/admin/players/:id/ban。
```

## 具体文件定位速查

如果你不知道每段代码该放在哪里，先按这个表定位。

### 1. 数据库结构文件

文件：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
```

怎么定位：

打开文件后，搜索：

```text
CREATE TABLE IF NOT EXISTS players
```

你要改的是 `players` 表这一整段。

再搜索：

```text
CREATE TABLE IF NOT EXISTS admin_operation_logs
```

确认 Day 09 的操作日志表还在，不要删。

### 2. 玩家模型文件

文件：

```text
E:\game-realtime-gm\backend\internal\model\player.go
```

怎么定位：

打开文件后，搜索：

```text
type Player struct
```

Day 10 是替换这个结构体所在文件的内容。

你最终要看到字段：

```text
Status
BannedReason
BannedAt
BannedByAdminID
```

### 3. 管理员业务文件

文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

这个文件今天会改三处。

第一处，搜索：

```text
type AdminHandler struct
```

在它后面放：

```go
type banPlayerRequest struct {
	Reason string `json:"reason" binding:"required"`
}
```

第二处，搜索：

```text
func (h *AdminHandler) recordOperation(
```

在这个函数结束后面放：

```text
recordOperationTx
```

第三处，直接到文件最底部。

在最后一个 `}` 后面新增：

```text
func (h *AdminHandler) BanPlayer(c *gin.Context)
```

如果你已经能搜到：

```text
func (h *AdminHandler) BanPlayer
```

说明这一段已经加过了，不要重复粘贴。

### 4. 玩家登录文件

文件：

```text
E:\game-realtime-gm\backend\internal\handler\auth.go
```

这个文件今天会改三处。

第一处，搜索：

```text
func (h *AuthHandler) Register
```

在这个函数里继续搜索：

```text
RETURNING id, username
```

这里要补 `status`、`banned_reason`、`banned_at`、`banned_by_admin_id`。

第二处，搜索：

```text
func (h *AuthHandler) Login
```

在这个函数里继续搜索：

```text
SELECT id, username, password_hash
```

这里也要补 `status`、`banned_reason`、`banned_at`、`banned_by_admin_id`。

第三处，搜索：

```text
CompareHashAndPassword
```

封禁判断要放在密码校验成功之后、生成 token 之前。

也就是放在：

```text
CompareHashAndPassword 代码块后面
GenerateToken 前面
```

### 5. 玩家查询文件

文件：

```text
E:\game-realtime-gm\backend\internal\handler\player.go
```

这个文件今天会改三处。

第一处，搜索：

```text
func (h *PlayerHandler) UpdateNickname
```

在这个函数里搜索：

```text
RETURNING id, username
```

这里要补玩家状态字段。

第二处，搜索：

```text
func (h *PlayerHandler) List
```

在这个函数里搜索：

```text
SELECT id, username
```

这里要补玩家状态字段。

第三处，搜索：

```text
func (h *PlayerHandler) findPlayerByID
```

在这个函数里搜索：

```text
SELECT id, username
```

这里也要补玩家状态字段。

### 6. 路由文件

文件：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

怎么定位：

搜索：

```text
adminProtected.GET("/players/:id", adminHandler.GetPlayerByID)
```

在这行下面新增：

```go
adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
```

最终这几行应该长这样：

```go
adminProtected.GET("/me", adminHandler.Me)
adminProtected.GET("/players", adminHandler.ListPlayers)
adminProtected.GET("/players/:id", adminHandler.GetPlayerByID)
adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
```

### 7. 如果你不确定自己加过没有

在 PowerShell 执行这些搜索：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "BanPlayer"
Select-String -Path E:\game-realtime-gm\backend\internal\router\router.go -Pattern "/players/:id/ban"
Select-String -Path E:\game-realtime-gm\backend\internal\model\player.go -Pattern "BannedReason"
Select-String -Path E:\game-realtime-gm\backend\internal\handler\auth.go -Pattern "player is banned"
```

如果某条命令没有任何输出，说明对应代码大概率还没加。

## 任务 0：确认 Day 09 状态

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

成功结果：

```text
?   	game-realtime-gm/backend/internal/handler	[no test files]
?   	game-realtime-gm/backend/internal/router	[no test files]
```

`[no test files]` 不是错误。

### 再确认操作日志表

Docker Desktop 进入 PostgreSQL：

```bash
psql -U game -d game_realtime
```

执行：

```sql
SELECT id, admin_username, action, target_type, target_id, detail, created_at
FROM admin_operation_logs
ORDER BY id DESC
LIMIT 10;
```

如果能看到 Day 09 的日志，说明可以继续。

## 任务 1：在 Docker Desktop 里给 players 表加封禁字段

### 在哪里操作

Docker Desktop 的 PostgreSQL 容器 Terminal。

进入数据库：

```bash
psql -U game -d game_realtime
```

看到：

```text
game_realtime=#
```

后执行下面 SQL。

### 执行 SQL

```sql
ALTER TABLE players
ADD COLUMN IF NOT EXISTS status VARCHAR(32) NOT NULL DEFAULT 'normal';

ALTER TABLE players
ADD COLUMN IF NOT EXISTS banned_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE players
ADD COLUMN IF NOT EXISTS banned_at TIMESTAMPTZ;

ALTER TABLE players
ADD COLUMN IF NOT EXISTS banned_by_admin_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_players_status
ON players (status);
```

### 成功结果

第一次执行通常看到：

```text
ALTER TABLE
ALTER TABLE
ALTER TABLE
ALTER TABLE
CREATE INDEX
```

如果字段已经存在，可能看到：

```text
NOTICE:  column "status" of relation "players" already exists, skipping
ALTER TABLE
```

这也算成功。

### 验证字段

执行：

```sql
\d players
```

应该能看到新增字段：

```text
status
banned_reason
banned_at
banned_by_admin_id
```

### 字段解释

`status`：

```text
玩家状态。
```

今天先用两个值：

```text
normal
banned
```

`banned_reason`：

```text
封禁原因。
```

`banned_at`：

```text
封禁时间。
```

正常玩家这里是 `NULL`。

`banned_by_admin_id`：

```text
是谁封禁的。
```

这里记录管理员 ID。

## 任务 2：把封禁字段补进 schema.sql

### 任务目标

刚才只是改了 Docker 里的 PostgreSQL。

现在要把表结构也补进项目文件：

```text
E:\game-realtime-gm\backend\internal\database\schema.sql
```

否则以后重新初始化数据库时，这些字段会丢。

### 推荐做法

把 `players` 表从现在的：

```sql
CREATE TABLE IF NOT EXISTS players (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    nickname VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

改成：

```sql
CREATE TABLE IF NOT EXISTS players (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    nickname VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'normal',
    banned_reason TEXT NOT NULL DEFAULT '',
    banned_at TIMESTAMPTZ,
    banned_by_admin_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_players_status
ON players (status);
```

### 注意

`CREATE TABLE IF NOT EXISTS players (...)` 不会自动给已经存在的表补字段。

所以真实数据库要执行任务 1 的 `ALTER TABLE`。

`schema.sql` 只是保存“新环境应该是什么结构”。

## 任务 3：更新 Player 模型

### 任务目标

修改：

```text
E:\game-realtime-gm\backend\internal\model\player.go
```

操作类型：整文件替换。

### 完整代码

```go
package model

import "time"

type Player struct {
	ID              int64      `json:"id"`
	Username        string     `json:"username"`
	PasswordHash    string     `json:"-"`
	Nickname        string     `json:"nickname"`
	Status          string     `json:"status"`
	BannedReason    string     `json:"banned_reason,omitempty"`
	BannedAt        *time.Time `json:"banned_at,omitempty"`
	BannedByAdminID *int64     `json:"banned_by_admin_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
```

### 代码解释

`Status`：

```text
正常玩家是 normal，被封玩家是 banned。
```

`BannedReason`：

```text
封禁原因。
```

`omitempty` 表示为空时 JSON 里可以不显示。

`BannedAt`：

```go
*time.Time
```

因为数据库里的 `banned_at` 可以是 `NULL`。

Go 里用指针表示：

```text
有封禁时间或没有封禁时间。
```

`BannedByAdminID`：

```go
*int64
```

同理，正常玩家没有封禁管理员，所以是空。

## 任务 4：在 admin.go 里新增事务版操作日志函数

### 为什么需要事务

Day 09 的查询日志失败时，接口仍然可以成功。

但 Day 10 是封禁玩家，属于真正改变状态的 GM 操作。

更稳的做法是：

```text
封禁玩家成功
操作日志也成功
事务提交
```

如果操作日志写失败，就不提交封禁。

这样不会出现：

```text
玩家被封了，但日志里没有记录是谁封的
```

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 新增请求结构体

在：

```go
type AdminHandler struct {
	db *pgxpool.Pool
}
```

下面加入：

```go
type banPlayerRequest struct {
	Reason string `json:"reason" binding:"required"`
}
```

### 新增事务版日志函数

在现有：

```go
func (h *AdminHandler) recordOperation(...)
```

后面加入：

```go
func (h *AdminHandler) recordOperationTx(c *gin.Context, tx pgx.Tx, action string, targetType string, targetID *int64, detail string) error {
	adminID, ok := middleware.CurrentAdminID(c)
	if !ok {
		return nil
	}

	var targetValue any
	if targetID != nil {
		targetValue = *targetID
	}

	_, err := tx.Exec(
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
	return err
}
```

### 为什么不直接改 Day 09 的 recordOperation

Day 09 的查询日志已经能工作。

今天新增一个事务版函数，让封禁这种关键操作使用它。

这样既不破坏 Day 09，又能让 Day 10 更严谨。

## 任务 5：在 admin.go 里新增 BanPlayer 方法

### 任务目标

新增方法：

```go
func (h *AdminHandler) BanPlayer(c *gin.Context)
```

用于处理：

```text
POST /api/admin/players/:id/ban
```

### 新增代码

把下面方法加到 `admin.go` 末尾：

```go
func (h *AdminHandler) BanPlayer(c *gin.Context) {
	playerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || playerID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40051,
			"message": "invalid player id",
		})
		return
	}

	var req banPlayerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40052,
			"message": "invalid request",
		})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40053,
			"message": "ban reason cannot be empty",
		})
		return
	}
	if len([]rune(reason)) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40054,
			"message": "ban reason is too long",
		})
		return
	}

	adminID, ok := middleware.CurrentAdminID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40116,
			"message": "admin identity missing",
		})
		return
	}

	tx, err := h.db.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50051,
			"message": "begin transaction failed",
		})
		return
	}
	defer tx.Rollback(c.Request.Context())

	var currentStatus string
	err = tx.QueryRow(
		c.Request.Context(),
		`SELECT status FROM players WHERE id = $1`,
		playerID,
	).Scan(&currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40451,
			"message": "player not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50052,
			"message": "query player status failed",
		})
		return
	}

	if currentStatus == "banned" {
		c.JSON(http.StatusConflict, gin.H{
			"code":    40951,
			"message": "player already banned",
		})
		return
	}

	var player model.Player
	err = tx.QueryRow(
		c.Request.Context(),
		`UPDATE players
		 SET status = 'banned',
		     banned_reason = $1,
		     banned_at = NOW(),
		     banned_by_admin_id = $2,
		     updated_at = NOW()
		 WHERE id = $3
		 RETURNING id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id`,
		reason,
		adminID,
		playerID,
	).Scan(
		&player.ID,
		&player.Username,
		&player.Nickname,
		&player.CreatedAt,
		&player.UpdatedAt,
		&player.Status,
		&player.BannedReason,
		&player.BannedAt,
		&player.BannedByAdminID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50053,
			"message": "ban player failed",
		})
		return
	}

	if err := h.recordOperationTx(c, tx, "admin.players.ban", "player", &playerID, "reason="+reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50054,
			"message": "record ban operation failed",
		})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50055,
			"message": "commit transaction failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ban player success",
		"data":    player,
	})
}
```

### 代码重点解释

```go
reason := strings.TrimSpace(req.Reason)
```

去掉封禁原因前后的空格。

```go
len([]rune(reason)) > 200
```

限制封禁原因最长 200 个中文或英文字符。

不要让一个接口写入超长文本。

```go
tx, err := h.db.Begin(...)
```

开启事务。

今天封禁玩家和写操作日志要么一起成功，要么一起失败。

```go
defer tx.Rollback(...)
```

如果中途任何地方 `return`，事务会回滚。

如果后面已经 `Commit` 成功，再执行 `Rollback` 不会影响已提交结果。

```go
SELECT status FROM players WHERE id = $1
```

先确认玩家存在，并检查是否已经被封禁。

```go
if currentStatus == "banned"
```

如果已经封禁，就返回冲突：

```text
40951 player already banned
```

```go
UPDATE players SET status = 'banned'
```

真正把玩家状态改为封禁。

```go
h.recordOperationTx(...)
```

在同一个事务里写操作日志。

```go
tx.Commit(...)
```

提交事务。

只有执行到这里，封禁和日志才真正保存到数据库。

## 任务 6：更新管理员查询玩家 SQL

### 为什么要改

Day 08 的管理员查询接口只查：

```text
id, username, nickname, created_at, updated_at
```

Day 10 后，GM 查询玩家时应该能看到玩家状态。

所以要把两个管理员查询 SQL 补上：

```text
status
banned_reason
banned_at
banned_by_admin_id
```

### 修改 ListPlayers 里的 SELECT

找到：

```go
`SELECT id, username, nickname, created_at, updated_at
```

改成：

```go
`SELECT id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
```

然后找到 `rows.Scan`，从：

```go
&player.ID,
&player.Username,
&player.Nickname,
&player.CreatedAt,
&player.UpdatedAt,
```

改成：

```go
&player.ID,
&player.Username,
&player.Nickname,
&player.CreatedAt,
&player.UpdatedAt,
&player.Status,
&player.BannedReason,
&player.BannedAt,
&player.BannedByAdminID,
```

### 修改 GetPlayerByID 里的 SELECT

同样把：

```go
`SELECT id, username, nickname, created_at, updated_at
```

改成：

```go
`SELECT id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
```

并在 `Scan` 里补上：

```go
&player.Status,
&player.BannedReason,
&player.BannedAt,
&player.BannedByAdminID,
```

## 任务 7：更新玩家登录逻辑，封禁后不能再登录

### 任务目标

修改：

```text
E:\game-realtime-gm\backend\internal\handler\auth.go
```

### 修改 Register 的 RETURNING

找到：

```sql
RETURNING id, username, nickname, created_at, updated_at
```

改成：

```sql
RETURNING id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
```

并在 `Scan` 里追加：

```go
&player.Status,
&player.BannedReason,
&player.BannedAt,
&player.BannedByAdminID,
```

### 修改 Login 的 SELECT

找到：

```sql
SELECT id, username, password_hash, nickname, created_at, updated_at
FROM players
WHERE username = $1
```

改成：

```sql
SELECT id, username, password_hash, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
FROM players
WHERE username = $1
```

并在 `Scan` 里追加：

```go
&player.Status,
&player.BannedReason,
&player.BannedAt,
&player.BannedByAdminID,
```

### 在密码校验成功后加封禁判断

找到：

```go
if err := bcrypt.CompareHashAndPassword([]byte(player.PasswordHash), []byte(req.Password)); err != nil {
	c.JSON(http.StatusUnauthorized, gin.H{
		"code":    40101,
		"message": "username or password is wrong",
	})
	return
}
```

在它后面加入：

```go
if player.Status == "banned" {
	c.JSON(http.StatusForbidden, gin.H{
		"code":    40321,
		"message": "player is banned",
		"data": gin.H{
			"reason": player.BannedReason,
		},
	})
	return
}
```

### 为什么放在密码校验之后

如果用户名存在但密码错误，仍然返回：

```text
username or password is wrong
```

只有密码正确后，才告诉用户：

```text
player is banned
```

这样不会让别人随便试一个用户名就知道这个账号是否被封。

## 任务 8：更新 player.go 查询字段

### 为什么要改

`Player` 模型多了字段。

如果某些查询仍然只查旧字段，响应里 `status` 可能是空字符串。

为了保持一致，要把玩家侧查询也补上状态字段。

### 修改 UpdateNickname 的 RETURNING

把：

```sql
RETURNING id, username, nickname, created_at, updated_at
```

改成：

```sql
RETURNING id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
```

并在 `Scan` 里补上：

```go
&player.Status,
&player.BannedReason,
&player.BannedAt,
&player.BannedByAdminID,
```

### 修改 List 的 SELECT

把：

```sql
SELECT id, username, nickname, created_at, updated_at
```

改成：

```sql
SELECT id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
```

并在 `rows.Scan` 里补上：

```go
&player.Status,
&player.BannedReason,
&player.BannedAt,
&player.BannedByAdminID,
```

### 修改 findPlayerByID 的 SELECT

把：

```sql
SELECT id, username, nickname, created_at, updated_at
```

改成：

```sql
SELECT id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
```

并在 `Scan` 里补上：

```go
&player.Status,
&player.BannedReason,
&player.BannedAt,
&player.BannedByAdminID,
```

## 任务 9：注册封禁路由

### 修改文件

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

### 修改位置

找到：

```go
adminProtected.GET("/players", adminHandler.ListPlayers)
adminProtected.GET("/players/:id", adminHandler.GetPlayerByID)
```

在下面新增：

```go
adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
```

### 修改后

```go
adminProtected.GET("/me", adminHandler.Me)
adminProtected.GET("/players", adminHandler.ListPlayers)
adminProtected.GET("/players/:id", adminHandler.GetPlayerByID)
adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
```

### 为什么用 POST

封禁玩家是状态变更操作。

不能用 GET。

GET 应该只用于查询。

这里用：

```text
POST /api/admin/players/:id/ban
```

表达：

```text
对这个玩家执行一次封禁动作。
```

## 任务 10：格式化并编译

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\model\player.go .\internal\handler\admin.go .\internal\handler\auth.go .\internal\handler\player.go .\internal\router\router.go
go test ./...
```

### 成功结果

看到：

```text
?   	game-realtime-gm/backend/internal/handler	[no test files]
?   	game-realtime-gm/backend/internal/router	[no test files]
```

说明编译通过。

### 常见编译错误

#### 1. `not enough arguments to Scan`

原因：

你改了 SELECT 字段数量，但没有同步修改 `Scan`。

规则是：

```text
SELECT 几个字段，Scan 就要接几个变量。
```

#### 2. `undefined: BanPlayer`

原因：

`router.go` 注册了：

```go
adminHandler.BanPlayer
```

但 `admin.go` 里没有成功新增：

```go
func (h *AdminHandler) BanPlayer(c *gin.Context)
```

#### 3. `undefined: banPlayerRequest`

原因：

你新增了 `BanPlayer`，但忘记加：

```go
type banPlayerRequest struct {
	Reason string `json:"reason" binding:"required"`
}
```

#### 4. `column "status" does not exist`

原因：

代码已经查 `status` 字段，但 Docker Desktop 的 PostgreSQL 还没有执行任务 1 的 `ALTER TABLE`。

解决：

回到任务 1 执行 SQL。

## 任务 11：启动后端

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

成功后：

```text
database connected
redis connected
server listening on :8080
```

如果出现：

```text
listen tcp :8080: bind: Only one usage of each socket address
```

说明旧服务还占着 8080。

找到旧的 PowerShell 或 GoLand 运行窗口，按：

```text
Ctrl + C
```

再重新启动。

## 任务 12：用 Apifox 封禁玩家

### 第 1 步：登录管理员

请求：

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

复制：

```text
data.token
```

### 第 2 步：找到一个玩家 ID

请求：

```text
GET http://localhost:8080/api/admin/players?page=1&page_size=10
```

Headers：

```text
Authorization: Bearer 管理员token
```

从 `data.items` 里找一个玩家 ID。

如果没有玩家，先注册一个：

```text
POST http://localhost:8080/api/register
```

Body：

```json
{
  "username": "testplayer",
  "password": "123456",
  "nickname": "测试玩家"
}
```

### 第 3 步：封禁玩家

假设玩家 ID 是 `1`：

```text
POST http://localhost:8080/api/admin/players/1/ban
```

Headers：

```text
Authorization: Bearer 管理员token
Content-Type: application/json
```

Body：

```json
{
  "reason": "测试封禁"
}
```

成功响应：

```json
{
  "code": 0,
  "message": "ban player success"
}
```

响应里的玩家状态应该是：

```json
"status": "banned"
```

## 任务 13：在 PostgreSQL 里验证玩家状态

Docker Desktop 进入 PostgreSQL：

```bash
psql -U game -d game_realtime
```

执行：

```sql
SELECT id, username, nickname, status, banned_reason, banned_at, banned_by_admin_id
FROM players
ORDER BY id DESC;
```

被封玩家应该类似：

```text
id = 1
username = testplayer
status = banned
banned_reason = 测试封禁
banned_at = 2026-06-10 ...
banned_by_admin_id = 1
```

## 任务 14：验证操作日志

继续在 PostgreSQL 里执行：

```sql
SELECT id, admin_username, action, target_type, target_id, detail, created_at
FROM admin_operation_logs
ORDER BY id DESC
LIMIT 10;
```

应该看到：

```text
action = admin.players.ban
target_type = player
target_id = 1
detail = reason=测试封禁
```

这说明封禁和日志都写成功了。

## 任务 15：验证被封玩家不能再登录

Apifox 请求：

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

预期返回：

```json
{
  "code": 40321,
  "message": "player is banned",
  "data": {
    "reason": "测试封禁"
  }
}
```

如果还能登录成功，重点检查：

- `auth.go` 的 Login SQL 有没有查 `status`。
- `Scan` 有没有接收 `player.Status`。
- 密码校验之后有没有加 `if player.Status == "banned"`。
- 后端服务有没有重启。

## 任务 16：测试权限隔离

### 不带 token 封禁

请求：

```text
POST http://localhost:8080/api/admin/players/1/ban
```

不带 Authorization。

预期：

```json
{
  "code": 40112,
  "message": "authorization header missing"
}
```

### 用玩家 token 封禁

先用普通玩家登录拿 token。

然后请求：

```text
POST http://localhost:8080/api/admin/players/1/ban
```

Headers：

```text
Authorization: Bearer 玩家token
```

预期：

```json
{
  "code": 40311,
  "message": "admin permission required"
}
```

### 用管理员 token 封禁

Headers：

```text
Authorization: Bearer 管理员token
```

预期成功。

这说明：

```text
只有管理员能封禁玩家。
```

## 常见问题

### 1. 封禁接口返回 404

原因：

玩家 ID 不存在。

先用：

```text
GET /api/admin/players?page=1&page_size=10
```

确认玩家 ID。

### 2. 封禁接口返回 409

原因：

玩家已经被封禁了。

今天还没有做解封，所以不能重复封禁同一个玩家。

### 3. 封禁接口返回 50054

原因：

玩家状态更新成功前，操作日志写入失败，事务回滚。

检查：

```text
admin_operation_logs 表是否存在
字段是否完整
```

### 4. PostgreSQL 显示时间和本地时间不一样

你可能看到：

```text
2026-06-10 08:xx:xx+00
```

这是 UTC 时间。

你在中国本地时间是 UTC+8。

例如：

```text
08:00 UTC = 16:00 Asia/Shanghai
```

这是正常现象。

### 5. 被封玩家旧 token 还能访问接口

这是 Day 10 的已知边界。

今天只做：

```text
封禁后不能再次登录
```

旧 token 立即失效需要后续增强：

```text
Auth 中间件每次查 players.status
或者 Redis 保存 token_version / banned 状态
```

后面再做会更稳。

## 今日验收清单

- [ ] Docker Desktop 的 PostgreSQL 已给 `players` 表加封禁字段。
- [ ] `schema.sql` 已保存封禁字段。
- [ ] `model.Player` 已加入 `status`、`banned_reason`、`banned_at`、`banned_by_admin_id`。
- [ ] `admin.go` 已新增 `banPlayerRequest`。
- [ ] `admin.go` 已新增 `recordOperationTx`。
- [ ] `admin.go` 已新增 `BanPlayer`。
- [ ] `router.go` 已注册 `POST /api/admin/players/:id/ban`。
- [ ] `auth.go` 登录时会检查 `player.Status == "banned"`。
- [ ] `go test ./...` 通过。
- [ ] 管理员 token 可以封禁玩家。
- [ ] 玩家表能看到 `status = banned`。
- [ ] 操作日志表能看到 `admin.players.ban`。
- [ ] 被封玩家再次登录返回 `40321 player is banned`。
- [ ] 玩家 token 或无 token 不能调用封禁接口。

## 请求链路复盘

### GM 封禁玩家

```text
Apifox
  ↓
POST /api/admin/players/:id/ban
  ↓
router.go
  ↓
middleware.AdminAuth
  ↓
AdminHandler.BanPlayer
  ↓
开启事务
  ↓
SELECT status FROM players
  ↓
UPDATE players SET status = 'banned'
  ↓
INSERT INTO admin_operation_logs
  ↓
COMMIT
  ↓
JSON response
```

### 被封玩家再次登录

```text
Apifox
  ↓
POST /api/login
  ↓
AuthHandler.Login
  ↓
SELECT player + status
  ↓
bcrypt 校验密码
  ↓
发现 status = banned
  ↓
返回 40321 player is banned
```

## 面试怎么讲这一部分

可以这样说：

```text
在 GM 查询玩家和操作日志之后，我实现了封禁玩家的基础能力。数据库 players 表新增 status、banned_reason、banned_at、banned_by_admin_id 字段，管理员通过 POST /api/admin/players/:id/ban 封禁玩家。封禁接口挂在 AdminAuth 后面，只允许管理员 token 调用。实现时使用事务保证玩家状态更新和 admin_operation_logs 操作日志一起成功或一起失败。玩家登录时会查询 status，若状态为 banned，则在密码验证通过后返回 403，阻止继续登录。这个功能把后台权限、状态变更、审计日志和登录控制串成了一个完整闭环。
```

## 明日预告

Day 11 建议做：

```text
GM 解封玩家基础
```

原因：

封禁之后一定要有解封。

Day 11 可以新增：

```text
POST /api/admin/players/:id/unban
```

并写入：

```text
action = admin.players.unban
```

这样 GM 对玩家状态的基础管理就完整了。
