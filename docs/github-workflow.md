# GitHub 提交与推送流程

本文档记录 `game-realtime-gm` 的日常 Git 工作流。

## 提交原则

- 一个提交只表达一个清晰目的。
- 提交前先验证代码和配置。
- 始终选择性暂存，不使用未经检查的 `git add .`。
- 不提交本地环境文件、凭据、数据库数据或私人材料。

## 提交前检查

```powershell
cd .
git status -sb
git diff --check

cd .\backend
go test ./...
go vet ./...

cd ..\deploy
docker compose config
```

涉及运行环境时，确认容器状态：

```powershell
docker compose ps
```

当前依赖容器为：

```text
game_realtime_mysql
game_realtime_redis
```

## 选择性暂存

返回项目根目录，根据实际修改添加文件：

```powershell
cd .
git add backend deploy README.md docs/api-overview.md
```

如果某个路径没有修改，不需要添加。暂存后必须检查：

```powershell
git diff --cached --name-status
git diff --cached
```

## 禁止提交

```text
.env
.env.*
*.pem
*.key
*.dump
*.sql.gz
*.log
真实账号和个人隐私
真实数据库密码
真实 JWT_SECRET
Docker volume 数据
编辑器临时文件
本地私人文档和原始资料
```

## 提交与推送

```powershell
git commit -m "Describe the completed change"
git push
```

推送后检查：

```powershell
git status -sb
git log --oneline -n 5
```

正常情况下，本地分支和 `origin/main` 不应存在未解释的领先或落后提交。
