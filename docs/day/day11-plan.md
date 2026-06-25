# Day 11 学习计划：GM 解封玩家基础

## 你今天从任务几开始

你现在从：

```text
任务 -1：先修 Day 10 注册接口的 Scan 重复问题
```

开始。

原因是我检查当前代码时发现：

```text
E:\game-realtime-gm\backend\internal\handler\auth.go
```

里的 `Register` 方法有一个运行时隐患：

```text
SQL RETURNING 返回 9 个字段，但 Scan 写了 13 个接收变量。
```

`go test ./...` 只能检查编译，不会真正执行注册 SQL，所以它现在能通过。但你后面如果注册新玩家，可能会出现：

```text
number of field descriptions must equal number of destinations
```

所以 Day 11 正式开始前，先把这个点修掉。

## 今日目标

今天做 GM 玩家状态管理的第二个动作：

```text
解封玩家
```

Day 10 已经做了：

```text
POST /api/admin/players/:id/ban
```

Day 11 新增：

```text
POST /api/admin/players/:id/unban
```

最终形成完整闭环：

```text
GM 封禁玩家
  ↓
玩家不能登录
  ↓
GM 解封玩家
  ↓
玩家可以重新登录
  ↓
封禁和解封都有操作日志
```

## 今日最终效果

管理员请求：

```text
POST http://localhost:8080/api/admin/players/1/unban
```

Headers：

```text
Authorization: Bearer 管理员token
Content-Type: application/json
```

Body：

```json
{
  "reason": "申诉通过"
}
```

成功响应类似：

```json
{
  "code": 0,
  "message": "unban player success",
  "data": {
    "id": 1,
    "username": "testplayer",
    "nickname": "测试玩家",
    "status": "normal"
  }
}
```

同时操作日志会新增：

```text
action = admin.players.unban
target_type = player
target_id = 1
detail = reason=申诉通过
```

玩家再次登录：

```text
POST http://localhost:8080/api/login
```

应该恢复成功。

## 今天会学到什么

今天会学到：

- 解封和封禁为什么是一对操作。
- 如何设计状态恢复 SQL。
- 如何把 `banned_reason`、`banned_at`、`banned_by_admin_id` 清空。
- 为什么解封也必须写操作日志。
- 为什么状态变更和操作日志要放在同一个事务。
- 如何处理“玩家本来就没被封”的业务冲突。

## 今日文件范围

今天建议修改这些文件：

```text
E:\game-realtime-gm\backend\internal\handler\auth.go
E:\game-realtime-gm\backend\internal\handler\admin.go
E:\game-realtime-gm\backend\internal\router\router.go
```

每个文件作用：

`auth.go`：

```text
先修 Day 10 注册接口 Scan 重复问题。
```

`admin.go`：

```text
新增 GM 解封玩家方法 UnbanPlayer。
```

`router.go`：

```text
注册 POST /api/admin/players/:id/unban。
```

今天不需要改：

```text
schema.sql
model/player.go
handler/player.go
```

原因是 Day 10 已经给 `players` 表和 `Player` 模型加过封禁字段。Day 11 只是把这些字段恢复为正常状态，不需要新增字段。

## 具体文件定位速查

### 1. 修注册 Scan 的位置

文件：

```text
E:\game-realtime-gm\backend\internal\handler\auth.go
```

搜索：

```text
func (h *AuthHandler) Register
```

然后在这个函数里搜索：

```text
RETURNING id, username
```

再看它下面的：

```text
).Scan(
```

这里需要确认 `Scan` 只接收 9 个变量。

### 2. 新增解封方法的位置

文件：

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

搜索：

```text
func (h *AdminHandler) BanPlayer
```

Day 11 的 `UnbanPlayer` 建议放在整个 `BanPlayer` 方法结束之后。

也就是：

```text
BanPlayer 最后的 } 后面
```

新增：

```text
func (h *AdminHandler) UnbanPlayer(c *gin.Context)
```

### 3. 注册路由的位置

文件：

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

搜索：

```text
adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
```

在这行下面新增：

```go
adminProtected.POST("/players/:id/unban", adminHandler.UnbanPlayer)
```

最终管理员路由应该类似：

```go
adminProtected.GET("/me", adminHandler.Me)
adminProtected.GET("/players", adminHandler.ListPlayers)
adminProtected.GET("/players/:id", adminHandler.GetPlayerByID)
adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
adminProtected.POST("/players/:id/unban", adminHandler.UnbanPlayer)
```

## 任务 -1：修 Day 10 注册接口 Scan 重复问题

### 为什么要先修

你当前 `auth.go` 里 `Register` 方法大概是这样的：

```go
RETURNING id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id
```

这句 SQL 返回 9 个字段。

所以 `Scan` 应该只有 9 个接收变量：

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

不要重复再写一遍：

```go
&player.Status,
&player.BannedReason,
&player.BannedAt,
&player.BannedByAdminID,
```

### 具体怎么改

打开：

```text
E:\game-realtime-gm\backend\internal\handler\auth.go
```

搜索：

```text
func (h *AuthHandler) Register
```

找到这个函数里的 `.Scan(`。

把 `Scan` 这一段整理成：

```go
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
```

### 修完后先编译

PowerShell 执行：

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\auth.go
go test ./...
```

如果通过，再继续 Day 11。

## 任务 0：确认 Day 10 状态

### 任务目标

确认封禁玩家功能已经存在。

### 检查代码

PowerShell 执行：

```powershell
Select-String -Path E:\game-realtime-gm\backend\internal\handler\admin.go -Pattern "BanPlayer"
Select-String -Path E:\game-realtime-gm\backend\internal\router\router.go -Pattern "/players/:id/ban"
Select-String -Path E:\game-realtime-gm\backend\internal\handler\auth.go -Pattern "player is banned"
```

如果三条都有输出，说明 Day 10 关键代码存在。

### 检查数据库字段

Docker Desktop 进入 PostgreSQL：

```bash
psql -U game -d game_realtime
```

执行：

```sql
\d players
```

确认有这些字段：

```text
status
banned_reason
banned_at
banned_by_admin_id
```

## 任务 1：理解解封要改哪些字段

封禁时，Day 10 做的是：

```sql
status = 'banned'
banned_reason = '测试封禁'
banned_at = NOW()
banned_by_admin_id = 管理员ID
```

解封时，要反过来：

```sql
status = 'normal'
banned_reason = ''
banned_at = NULL
banned_by_admin_id = NULL
updated_at = NOW()
```

也就是说，解封不是删除玩家，也不是删除日志。

解封只是把玩家状态恢复正常，并清空当前封禁信息。

历史封禁和解封记录保存在：

```text
admin_operation_logs
```

## 任务 2：新增解封请求结构体

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 具体位置

搜索：

```text
type banPlayerRequest struct
```

你现在应该能看到：

```go
type banPlayerRequest struct {
	Reason string `json:"reason" binding:"required"`
}
```

在它下面新增：

```go
type unbanPlayerRequest struct {
	Reason string `json:"reason" binding:"required"`
}
```

### 为什么解封也要 reason

解封也是 GM 操作，也需要审计原因。

比如：

```text
误封解除
申诉通过
处罚期结束
```

后面查日志时才能知道为什么解封。

## 任务 3：新增 UnbanPlayer 方法

### 修改文件

```text
E:\game-realtime-gm\backend\internal\handler\admin.go
```

### 具体位置

搜索：

```text
func (h *AdminHandler) BanPlayer
```

一直往下找到 `BanPlayer` 方法最后的：

```go
}
```

在它后面新增下面完整方法。

### 新增代码

```go
func (h *AdminHandler) UnbanPlayer(c *gin.Context) {
	playerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || playerID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40061,
			"message": "invalid player id",
		})
		return
	}

	var req unbanPlayerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40062,
			"message": "invalid request",
		})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40063,
			"message": "unban reason cannot be empty",
		})
		return
	}
	if len([]rune(reason)) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40064,
			"message": "unban reason is too long",
		})
		return
	}

	tx, err := h.db.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50061,
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
			"code":    40461,
			"message": "player not found",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50062,
			"message": "query player status failed",
		})
		return
	}

	if currentStatus != "banned" {
		c.JSON(http.StatusConflict, gin.H{
			"code":    40961,
			"message": "player is not banned",
		})
		return
	}

	var player model.Player
	err = tx.QueryRow(
		c.Request.Context(),
		`UPDATE players
		 SET status = 'normal',
		     banned_reason = '',
		     banned_at = NULL,
		     banned_by_admin_id = NULL,
		     updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, username, nickname, created_at, updated_at, status, banned_reason, banned_at, banned_by_admin_id`,
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
			"code":    50063,
			"message": "unban player failed",
		})
		return
	}

	if err := h.recordOperationTx(c, tx, "admin.players.unban", "player", &playerID, "reason="+reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50064,
			"message": "record unban operation failed",
		})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50065,
			"message": "commit transaction failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "unban player success",
		"data":    player,
	})
}
```

## 任务 4：代码逐段理解

### 1. 校验玩家 ID

```go
playerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
```

从 URL 里取玩家 ID。

例如：

```text
POST /api/admin/players/1/unban
```

这里的 `1` 会变成 `playerID`。

### 2. 校验解封原因

```go
reason := strings.TrimSpace(req.Reason)
```

去掉前后空格。

```go
if reason == ""
```

不允许空原因。

```go
len([]rune(reason)) > 200
```

限制最多 200 个字符，避免写入超长文本。

### 3. 开启事务

```go
tx, err := h.db.Begin(c.Request.Context())
```

解封玩家和写操作日志必须在同一个事务里。

目标是：

```text
玩家状态恢复 normal
操作日志写入 admin.players.unban
```

要么一起成功，要么一起失败。

### 4. 检查当前状态

```go
SELECT status FROM players WHERE id = $1
```

先确认玩家存在。

如果不存在，返回：

```text
40461 player not found
```

如果玩家不是 banned，返回：

```text
40961 player is not banned
```

### 5. 恢复玩家状态

```sql
SET status = 'normal',
    banned_reason = '',
    banned_at = NULL,
    banned_by_admin_id = NULL
```

这表示当前封禁状态已经解除。

注意：

```text
不是删除历史日志。
```

历史记录仍然在 `admin_operation_logs` 里。

### 6. 写操作日志

```go
h.recordOperationTx(c, tx, "admin.players.unban", "player", &playerID, "reason="+reason)
```

写入解封日志。

这条日志回答：

```text
谁在什么时候因为什么解封了哪个玩家。
```

## 任务 5：注册解封路由

### 修改文件

```text
E:\game-realtime-gm\backend\internal\router\router.go
```

### 具体位置

搜索：

```text
adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
```

在它下面新增：

```go
adminProtected.POST("/players/:id/unban", adminHandler.UnbanPlayer)
```

### 修改后应该是

```go
adminProtected.GET("/me", adminHandler.Me)
adminProtected.GET("/players", adminHandler.ListPlayers)
adminProtected.GET("/players/:id", adminHandler.GetPlayerByID)
adminProtected.POST("/players/:id/ban", adminHandler.BanPlayer)
adminProtected.POST("/players/:id/unban", adminHandler.UnbanPlayer)
```

## 任务 6：格式化并编译

### 在哪里执行

PowerShell。

### 执行命令

```powershell
cd E:\game-realtime-gm\backend
gofmt -w .\internal\handler\auth.go .\internal\handler\admin.go .\internal\router\router.go
go test ./...
```

### 成功结果

看到类似：

```text
?   	game-realtime-gm/backend/internal/handler	[no test files]
?   	game-realtime-gm/backend/internal/router	[no test files]
```

### 常见错误

#### 1. `undefined: unbanPlayerRequest`

原因：

你新增了 `UnbanPlayer` 方法，但忘记加：

```go
type unbanPlayerRequest struct {
	Reason string `json:"reason" binding:"required"`
}
```

#### 2. `undefined: adminHandler.UnbanPlayer`

原因：

`router.go` 注册了解封路由，但 `admin.go` 里没有 `UnbanPlayer` 方法。

#### 3. `undefined: pgx`

原因：

`UnbanPlayer` 里用了：

```go
errors.Is(err, pgx.ErrNoRows)
```

但 `admin.go` 的 import 里缺少：

```go
"github.com/jackc/pgx/v5"
```

你当前项目里 `admin.go` 已经有这个 import，正常不需要再加。

## 任务 7：启动后端

PowerShell 执行：

```powershell
cd E:\game-realtime-gm\backend
go run .\cmd\server
```

成功看到：

```text
database connected
redis connected
server listening on :8080
```

如果出现：

```text
listen tcp :8080: bind: Only one usage of each socket address
```

说明 8080 已经有后端进程。

先找到旧的 PowerShell 或 GoLand 运行窗口，按：

```text
Ctrl + C
```

再重新启动。

## 任务 8：先封禁一个玩家

如果你已经有被封玩家，可以跳过这一步。

### 登录管理员

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

复制：

```text
data.token
```

### 查询玩家列表

```text
GET http://localhost:8080/api/admin/players?page=1&page_size=10
```

Headers：

```text
Authorization: Bearer 管理员token
```

复制一个玩家 ID。

### 封禁玩家

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
  "reason": "Day11 解封前测试封禁"
}
```

成功后玩家状态应该是：

```text
banned
```

## 任务 9：调用解封接口

Apifox 请求：

```text
POST http://localhost:8080/api/admin/players/1/unban
```

Headers：

```text
Authorization: Bearer 管理员token
Content-Type: application/json
```

Body：

```json
{
  "reason": "申诉通过"
}
```

成功响应：

```json
{
  "code": 0,
  "message": "unban player success",
  "data": {
    "id": 1,
    "username": "testplayer",
    "nickname": "测试玩家",
    "status": "normal"
  }
}
```

## 任务 10：在 PostgreSQL 里验证玩家状态

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

解封后的玩家应该是：

```text
status = normal
banned_reason = 空字符串
banned_at = NULL
banned_by_admin_id = NULL
```

如果你在 `psql` 查询结果底部看到：

```text
(END)
```

按：

```text
q
```

退出分页界面。

## 任务 11：验证操作日志

继续在 PostgreSQL 里执行：

```sql
SELECT id, admin_username, action, target_type, target_id, detail, created_at
FROM admin_operation_logs
ORDER BY id DESC
LIMIT 10;
```

应该看到：

```text
action = admin.players.unban
target_type = player
target_id = 1
detail = reason=申诉通过
```

你也应该能看到之前的：

```text
action = admin.players.ban
```

这样封禁和解封都有审计记录。

## 任务 12：验证玩家可以重新登录

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

如果解封成功，应该返回：

```json
{
  "code": 0,
  "message": "login success"
}
```

如果仍然返回：

```json
{
  "code": 40321,
  "message": "player is banned"
}
```

检查数据库里这个玩家的：

```text
status
```

是否真的变成：

```text
normal
```

## 任务 13：测试异常情况

### 1. 解封不存在的玩家

请求：

```text
POST http://localhost:8080/api/admin/players/999999/unban
```

预期：

```json
{
  "code": 40461,
  "message": "player not found"
}
```

### 2. 解封一个正常玩家

如果玩家本来就是：

```text
normal
```

再请求解封，预期：

```json
{
  "code": 40961,
  "message": "player is not banned"
}
```

### 3. 不带 token 解封

请求：

```text
POST http://localhost:8080/api/admin/players/1/unban
```

不带 Authorization。

预期：

```json
{
  "code": 40112,
  "message": "authorization header missing"
}
```

### 4. 用玩家 token 解封

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

## 常见问题

### 1. 解封接口返回 404

原因：

玩家 ID 不存在。

先请求：

```text
GET /api/admin/players?page=1&page_size=10
```

确认真实玩家 ID。

### 2. 解封接口返回 409

原因：

玩家当前不是 `banned`。

先在 PostgreSQL 里查：

```sql
SELECT id, username, status FROM players ORDER BY id DESC;
```

如果是 `normal`，就不能解封。

### 3. 解封成功但日志没有

按今天设计，解封和写日志在一个事务里。

如果接口已经返回成功，理论上日志应该存在。

如果没有，检查你是否在 `UnbanPlayer` 里调用了：

```go
h.recordOperationTx(c, tx, "admin.players.unban", "player", &playerID, "reason="+reason)
```

### 4. 解封后登录仍然失败

检查数据库：

```sql
SELECT username, status, banned_reason FROM players WHERE id = 你的玩家ID;
```

如果 `status` 还是 `banned`，说明解封没有真正更新。

如果 `status` 是 `normal`，但登录仍然失败，检查后端是否重启。

## 今日验收清单

- [ ] 已修复 `auth.go` 注册接口 `Scan` 重复问题。
- [ ] `go test ./...` 通过。
- [ ] `admin.go` 已新增 `unbanPlayerRequest`。
- [ ] `admin.go` 已新增 `UnbanPlayer` 方法。
- [ ] `router.go` 已注册 `POST /api/admin/players/:id/unban`。
- [ ] 管理员 token 可以解封 banned 玩家。
- [ ] 正常玩家不能重复解封，返回 40961。
- [ ] 不存在玩家返回 40461。
- [ ] 无 token 或玩家 token 不能调用解封接口。
- [ ] 解封后 `players.status = normal`。
- [ ] 解封后 `banned_reason`、`banned_at`、`banned_by_admin_id` 被清空。
- [ ] 操作日志表出现 `admin.players.unban`。
- [ ] 解封后玩家可以重新登录。

## 请求链路复盘

### GM 解封玩家

```text
Apifox
  ↓
POST /api/admin/players/:id/unban
  ↓
router.go
  ↓
middleware.AdminAuth
  ↓
AdminHandler.UnbanPlayer
  ↓
开启事务
  ↓
SELECT status FROM players
  ↓
UPDATE players SET status = 'normal'
  ↓
INSERT INTO admin_operation_logs
  ↓
COMMIT
  ↓
JSON response
```

### 解封后登录

```text
Apifox
  ↓
POST /api/login
  ↓
AuthHandler.Login
  ↓
查询 players.status
  ↓
密码校验通过
  ↓
status = normal
  ↓
生成玩家 token
  ↓
login success
```

## 面试怎么讲这一部分

可以这样讲：

```text
在封禁玩家功能之后，我继续实现了解封玩家。解封接口是 POST /api/admin/players/:id/unban，挂在 AdminAuth 管理员鉴权后面。接口会先校验玩家存在并且当前状态是 banned，然后在事务中把 players.status 改回 normal，清空 banned_reason、banned_at 和 banned_by_admin_id，同时向 admin_operation_logs 写入 admin.players.unban 操作日志。事务保证了解封状态更新和审计日志写入要么一起成功，要么一起失败。解封后玩家登录逻辑会看到 status = normal，因此可以重新登录。
```

## 明日预告

Day 12 建议做：

```text
GM 操作日志查询接口
```

原因：

现在系统已经会写这些日志：

```text
admin.players.list
admin.players.detail
admin.players.ban
admin.players.unban
```

下一步应该让 GM 能查询这些操作日志，例如：

```text
GET /api/admin/operation-logs
```

这样后台管理能力会更完整。
