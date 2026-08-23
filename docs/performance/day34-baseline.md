# Day34 WebSocket 本机性能基线

> 状态：实测记录
> 测试日期：2026-08-23
> 适用范围：本地学习环境 / 单实例 Go 服务
> 真实性边界：不是生产容量、商业 CCU 或 SLO 证明

## 代码版本

- 测试代码状态：Day34 待提交工作区，最终版本以本报告所在公开提交为准。
- 公开仓库测试前基线：`aa6fa18e9c626d2ee93795830120ecf39b4b79f9`。
- Windows Go：`go1.25.6 windows/amd64`。
- Race 容器 Go：`go1.25.14 linux/amd64`。
- 操作系统：Windows 11 专业版，build 26200。
- CPU：AMD Ryzen 5 7535H with Radeon Graphics，12 logical processors。
- 可见内存：15.24 GB。
- Docker Engine：29.7.2，Linux/amd64。
- Docker Compose：v5.4.0。
- MySQL：8.4.11。
- Redis：7。

## 服务配置

- `APP_PORT=18087`。
- `PPROF_ENABLED=true`。
- `PPROF_ADDR=127.0.0.1:16060`。
- Gin：debug mode。
- MySQL 和 Redis：本机 Docker Desktop。
- 客户端与服务端：同一台 Windows 机器、回环网络。
- 原始 CPU、heap 和 goroutine 文件保存在仓库外的本地目录，不提交 Git。

## 验证前修复

真实冒烟暴露并修复了两个问题：

1. `squad.Manager.Leave` 已计算 `nextMembers`，但没有写回 `s.Members`，导致离队响应成功后小队统计仍残留。修复后增加了成员移除和最后一人解散回归测试。
2. `ws_bot` 的 `echo-interval` 复用了带 phase 日志的等待函数，10000 次 echo 会产生约 10000 行客户端日志。修复后 interval 使用静默 timer，只保留阶段日志。

访问日志同时完成了安全调整：只记录 URL path，不记录 query；服务端不再逐条记录原始 WebSocket payload。实际终端输出中未出现 `?token=` 或 JWT。

## 冒烟参数

```powershell
go run .\cmd\tools\ws_bot `
  -http-url http://127.0.0.1:18087 `
  -ws-url ws://127.0.0.1:18087/ws `
  -username-prefix day34bot `
  -clients 8 `
  -squad-size 4 `
  -echo-rounds 10 `
  -warmup 2s `
  -hold 15s
```

## 冒烟结果

| Stage | Success | Failure | Average | P95 | Maximum |
| --- | ---: | ---: | ---: | ---: | ---: |
| `register` | 8 | 0 | 155.859 ms | 162.616 ms | 162.616 ms |
| `login` | 8 | 0 | 68.464 ms | 72.375 ms | 72.375 ms |
| `ws_connect` | 8 | 0 | 10.931 ms | 13.287 ms | 13.287 ms |
| `squad_create` | 2 | 0 | 766 us | 766 us | 766 us |
| `squad_join` | 6 | 0 | 318 us | 694 us | 694 us |
| `squad_ready` | 6 | 0 | 0 s | 0 s | 0 s |
| `debug_echo` | 80 | 0 | 192 us | 556 us | 556 us |
| `squad_leave` | 8 | 0 | 295 us | 655 us | 655 us |

Windows 本机回环请求偶尔在同一计时刻度内完成，`time.Since` 会返回 0。表中的 `0 s` 表示低于本次计时分辨率，不表示没有执行服务端逻辑。

冒烟业务数量全部符合预期，修复 `Leave` 后 cleanup 摘要为 0 连接、0 小队、0 成员。

## 20 客户端负载参数

```powershell
go run .\cmd\tools\ws_bot `
  -http-url http://127.0.0.1:18087 `
  -ws-url ws://127.0.0.1:18087/ws `
  -username-prefix day34bot `
  -clients 20 `
  -squad-size 4 `
  -echo-rounds 500 `
  -ensure-accounts=false `
  -warmup 5s `
  -echo-interval 20ms `
  -hold 25s
```

## 20 客户端结果

总持续时间：40.521 秒。

| Stage | Success | Failure | Average | P95 | Maximum |
| --- | ---: | ---: | ---: | ---: | ---: |
| `login` | 20 | 0 | 75.153 ms | 98.181 ms | 104.777 ms |
| `ws_connect` | 20 | 0 | 4.406 ms | 4.901 ms | 5.429 ms |
| `squad_create` | 5 | 0 | 0 s | 0 s | 0 s |
| `squad_join` | 15 | 0 | 283 us | 539 us | 539 us |
| `squad_ready` | 15 | 0 | 176 us | 539 us | 539 us |
| `debug_echo` | 10000 | 0 | 106 us | 611 us | 2.513 ms |
| `squad_leave` | 20 | 0 | 315 us | 755 us | 755 us |

收到的主要消息：

```text
server.welcome=20
squad.create.result=5
squad.join.result=15
squad.ready.result=15
debug.echo.result=10000
squad.leave.result=20
squad.state.changed=90
```

这些时间是本机客户端观察到的 HTTP/WebSocket 往返时间，不是服务端函数纯处理时间，也不能外推为生产容量。

## GM 摘要与 goroutine 对照

### Hold 期间

```text
online_connections=20
squads=5
members=20
online_members=20
goroutines=70
working_set=30.36 MB
threads=21
```

### Cleanup 后

```text
online_connections=0
squads=0
members=0
online_members=0
goroutines=8
```

goroutine 在连接关闭后从 70 回落到 8，本次观察没有发现连接 goroutine 持续残留。该结论只覆盖本次短时运行。

## CPU Profile

- 采样时间：15 秒。
- 总 CPU samples：160 ms，占采样窗口 1.07%。
- 采样从账号登录完成后的 warmup 开始，覆盖 echo 和部分 hold，没有被 bcrypt 主导。

| Node | Flat | Flat % | Cumulative |
| --- | ---: | ---: | ---: |
| `runtime.cgocall` | 70 ms | 43.75% | 70 ms |
| `router.New.WebSocketEcho.func5` | 10 ms | 6.25% | 110 ms |
| `memeqbody` | 10 ms | 6.25% | 10 ms |
| `runtime.(*mheap).tryAllocMSpan` | 10 ms | 6.25% | 10 ms |
| `runtime.(*timers).siftDown` | 10 ms | 6.25% | 10 ms |
| `runtime.(*waitq).dequeue` | 10 ms | 6.25% | 10 ms |
| `runtime.findRunnable` | 10 ms | 6.25% | 20 ms |
| `runtime.memmove` | 10 ms | 6.25% | 10 ms |
| `runtime.stdcall1` | 10 ms | 6.25% | 10 ms |
| `runtime/pprof.(*profMap).lookup` | 10 ms | 6.25% | 20 ms |

本次负载 CPU 使用较低，采样量只有 160 ms；不能据此确定稳定热点，也没有证据支持立即进行底层性能重构。

## Heap Profile

### `inuse_space`

快照总量约 7360.32 kB。主要可见节点：

| Node | In-use |
| --- | ---: |
| `runtime.allocm` | 2052 kB |
| `runtime/pprof.StartCPUProfile` | 1184.27 kB |
| `bufio.NewReaderSize` | 1042.17 kB |
| `encoding/xml.map.init.0` | 521.05 kB |
| `runtime.makeProfStackFP` | 512.56 kB |
| `net/http.readRequest` | 512.16 kB |
| `mysqlConn.startWatcher.func1` | 512.05 kB |
| `bufio.NewWriterSize` | 512.03 kB |
| `syscall.RawSockaddrAny.Sockaddr` | 512.03 kB |

### `alloc_space`

累计分配约 27.79 MB。主要可见节点：

| Node | Allocated |
| --- | ---: |
| `io.ReadAll` | 6.00 MB |
| `encoding/json.Unmarshal` | 2.50 MB |
| `runtime.allocm` | 2.00 MB |
| `router.New.WebSocketEcho.func5` | 2.00 MB flat / 12.50 MB cumulative |
| `compress/flate.NewWriter` | 1.76 MB |
| `compress/flate.(*compressor).init` | 1.27 MB |
| `runtime/pprof.StartCPUProfile` | 1.16 MB |
| `compress/flate.newDeflateFast` | 1.06 MB |
| `bufio.NewReaderSize` | 1.02 MB |
| `bufio.NewWriterSize` | 1.00 MB |

这些节点包含 Go runtime、HTTP、pprof 自身和 WebSocket 流程的分配。当前仅作为后续相同参数复测的基线，不根据一次 heap 快照直接优化。

## Race 结果

Windows 本机为 `CGO_ENABLED=0` 且没有 GCC，无法原生执行 race。使用官方 Go Docker 镜像完成：

```powershell
docker run --rm `
  -v "${PWD}:/workspace" `
  -w /workspace `
  golang:1.25-bookworm `
  go test -race ./...
```

结果：全部包通过，退出码 0。

```text
go version go1.25.14 linux/amd64
image digest sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437
image size 289799996 bytes
```

Race 只覆盖单元测试实际执行到的路径，不代表真实 WebSocket 运行流程的所有并发路径都经过检测。

## 日志安全检查

- AccessLog 测试确认 query token 不进入访问日志：通过。
- 实际服务日志只显示 `path=/ws`，没有 `?token=`：通过。
- 服务端不再逐条打印原始 echo payload：通过。
- `ws_bot` 不打印 token：通过。
- 原始 profile 和 goroutine 文件保存在仓库外：通过。

## 数据清理

- 删除 Day34 专用玩家：20 个。
- 删除对应零余额 `player_assets`：20 行。
- 对应 reward、ledger、submitted mission：均为 0。
- Day34 online key：最终无残留。
- `player01`、`player02` 余额：均为 0。
- `mission_records`、`reward_records`、`asset_ledger`：均为 0。

## 结论

- 8 人冒烟和 20 人、10000 echo 本机基线均为 0 失败。
- 小队 create/join/ready/leave 与 Day33 GM 摘要能够相互验证。
- 修复后离队会真正移除成员并在最后一人离开时解散小队。
- 20 个连接关闭后，连接、小队和 goroutine 数量回落。
- 当前没有发现需要立即进行底层性能重构的证据。

## 局限

- 单机、单进程、回环网络。
- 客户端和服务端共享一台机器。
- 只覆盖登录、WebSocket、小队和 `debug.echo`。
- 没有覆盖公网延迟、弱网、长时间 soak、多实例或数据库高负载。
- 8/20 客户端不代表容量上限。
- Gin 运行在 debug mode。
- pprof 和 race 结论只适用于本次命令和覆盖路径。

## 后续对比规则

后续优化必须记录机器、代码版本、Go 版本、服务配置和完整 `ws_bot` 参数，并使用相同命令复测后再比较。
