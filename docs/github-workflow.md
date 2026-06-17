# GitHub 日常提交与推送流程

本文档记录 `game-realtime-gm` 项目的 GitHub 工作流。

当前远程仓库：

```text
https://github.com/lki01/game-realtime-gm.git
```

当前主分支：

```text
main
```

## 每天什么时候提交

建议节奏：

```text
当天任务完成
        ↓
本地验证通过
        ↓
选择性 git add
        ↓
git commit
        ↓
git push
```

也就是：

```text
每天做完当天任务后 commit。
代码稳定后 push 到 GitHub。
```

不要每改一点就推送，也不要攒很多天才推送。

每天建议：

```text
1 个主 commit
必要时 2 个 commit
```

## 每次提交前检查

### 1. 查看当前状态

```powershell
cd E:\game-realtime-gm
git status
```

重点看：

```text
Changes to be committed
Changes not staged for commit
Untracked files
```

不要急着 `git add .`。

### 2. 后端编译测试

```powershell
cd E:\game-realtime-gm\backend
go test ./...
```

通过后再提交。

如果失败，先修复或记录原因，不要把明显无法编译的代码推到 GitHub。

### 3. Docker 状态检查

涉及数据库、Redis、Docker 的任务，检查：

```powershell
cd E:\game-realtime-gm\deploy
docker ps
```

应该能看到：

```text
game_realtime_postgres
game_realtime_redis
```

## 选择性添加文件

推荐添加方式：

```powershell
cd E:\game-realtime-gm
git add backend deploy README.md docs/api-overview.md docs/dayXX-plan.md
```

如果当天只改了某些文件，就只添加那些文件。

例如 Day16 只改了后端和 Day16 文档：

```powershell
git add backend docs/day16-plan.md docs/api-overview.md README.md
```

如果 `README.md` 和 `api-overview.md` 没变，不需要添加它们。

## 不建议使用 git add .

当前项目里有一些内部协作资料，不建议上传到 GitHub：

```text
claude.md
docs/codex-context.md
docs/conversation-handoff-gpt55.md
docs/mcp-adoption-plan.md
docs/skill-adoption-plan.md
```

所以不要无脑执行：

```powershell
git add .
```

除非你已经确认 `git status` 里没有不该上传的文件。

## 不要上传的内容

每次提交前检查不要上传：

```text
.env
.env.*
*.pem
*.key
*.dump
*.sql.gz
*.log
真实手机号
真实邮箱
真实数据库密码
真实 JWT_SECRET
Docker volume 数据
GoLand 临时文件
```

本地学习账号可以写在 README 中，但如果未来部署公网环境，必须更换默认密码和 JWT secret。

## 提交命令

添加文件后检查：

```powershell
git status
```

确认暂存区里只有你要提交的内容后：

```powershell
git commit -m "Complete dayXX ..."
```

示例：

```powershell
git commit -m "Complete day16 dashboard stats"
git commit -m "Update API overview after day16"
git commit -m "Fix operation log detail query"
```

提交后查看：

```powershell
git log --oneline -n 5
```

## 推送命令

```powershell
git push
```

如果是第一次推送某个新分支：

```powershell
git push -u origin 分支名
```

当前主分支已经跟踪 `origin/main`，日常直接：

```powershell
git push
```

即可。

## 推送后检查

```powershell
git status -sb
```

理想状态：

```text
## main...origin/main
```

如果还有未提交文件，要判断它们是不是故意保留的内部文件。

目前允许保留未提交的内部文件：

```text
claude.md
docs/codex-context.md
docs/conversation-handoff-gpt55.md
docs/mcp-adoption-plan.md
docs/skill-adoption-plan.md
```

## GitHub 网络问题

如果推送时报：

```text
Failed to connect to github.com port 443
```

说明是网络连不上 GitHub，不是项目代码问题。

处理方式：

1. 浏览器打开 GitHub 仓库确认能访问。
2. 稍后重试：

```powershell
git push
```

3. 如果使用代理，确认 Git 也配置了代理。

清理错误代理：

```powershell
git config --global --unset http.proxy
git config --global --unset https.proxy
```

## 适合展示给 HR 的 GitHub 状态

仓库首页应该重点展示：

```text
README.md
backend/
deploy/
docs/api-overview.md
docs/dayXX-plan.md
```

提交记录应该体现：

```text
持续迭代
每天一个小功能
验证后提交
文档和代码同步更新
```

这个项目不要包装成已经商用的大型游戏服务器。更准确的表达是：

```text
持续迭代中的 Go 游戏后台与 GM 运营后台学习项目。
```
