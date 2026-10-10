# GitHub 提交与公开发布流程

> 文档角色：安全提交、精确发布与公开边界操作手册
> 权威级别：L2（协作流程）
> 状态：已生效
> 适用范围：本地源工作区与干净公开工作区
> 事实来源：当前双工作区发布方式与文档治理规则
> 最后更新：2026-08-31

## 1. 两类工作区

- 源工作区：保留完整代码、未提交学习内容和私人文档，不因发布而 reset/clean。
- 公开干净工作区：只接收审查后的代码和 [公开索引](public-index.md) 所列文档，用于 commit/push。

不要从脏源工作区直接 `git add .` 或推送，也不要为了同步公开内容回退用户本地修改。

## 2. 提交前冻结状态

```powershell
git status -sb
git diff --check
git diff --stat
```

逐个确认改动来源。发现不属于本次任务的用户文件时不暂存、不回退。

后端相关变更至少执行：

```powershell
cd .\backend
$goFiles = Get-ChildItem .\cmd,.\internal -Recurse -Filter *.go | Select-Object -ExpandProperty FullName
gofmt -d $goFiles
go test ./...
go vet ./...
```

文档变更还要检查 OpenAPI、Markdown 链接、围栏、路由一致性和公开敏感信息。

## 3. 禁止上传

```text
.env / .env.*
*.pem / *.key
JWT、GitHub token、数据库真实密码
账号备忘、真实个人信息
*.dump / *.sql.gz / Docker volume
*.log / profile / goroutine dump
本机绝对路径与内部交接资料
非公开研究、历史学习、学术整理和内部协作材料
```

代码中的本地开发回退配置必须明确标注非生产；共享环境使用外部 Secret。

## 4. 精确同步公开材料

公开文档清单由 `docs/public-index.md` 和 `docs/documentation-governance.md` 决定。同步时：

1. 从源工作区读取已验证文件。
2. 只复制批准路径到干净公开工作区。
3. README 只链接公开工作区真实存在的文件。
4. 扫描非公开材料关键词、旧私有提交标识、token/Secret 样式和绝对路径。
5. 用 `git status -sb` 确认没有意外新增或删除。

## 5. 精确暂存

```powershell
git add README.md docs/public-index.md docs/openapi.yaml
git add docs/architecture.md docs/system-design.md
```

命令仅为示例；只添加本次实际变更。使用：

```powershell
git diff --cached --stat
git diff --cached --check
git diff --cached
```

不建议 `git add .`，因为它会把未知日志、私人文档或临时文件一并暂存。

## 6. 提交和推送

```powershell
git commit -m "Restructure authoritative project documentation"
git push origin main
git status -sb
```

理想状态：本地分支与远程一致、工作区干净。推送前使用 `gh auth status` 确认认证，不在终端输出 token。

不得无意执行 force push、历史重写、删除远程分支或覆盖他人提交。确需历史清理时必须作为独立任务，先备份和确认影响。

## 7. 推送后验证

- 远程 commit SHA 与本地 HEAD 一致。
- README、相对链接、Mermaid 和 OpenAPI 在 GitHub 可读。
- 公开树中不存在私人目录/文件。
- GitHub 默认分支与预期一致。
- 当前 GitHub Actions 包含主项目与独立 R5 两套工作流；逐项核对最终 PR head 和合并 main 的真实结果。CI 验证不表示自动部署。

## 8. 提交表达

提交信息描述真实结果，例如：

```text
Document phase-one architecture and contracts
Add OpenAPI and local operations runbooks
Fix WebSocket access-log redaction test
```

避免“production ready”“million QPS”“microservice platform”等没有证据的描述。
