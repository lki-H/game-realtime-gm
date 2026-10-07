# R3发布验收

> 权威级别：L2（发布证据摘要）
> 状态：R3原分支CI通过；本次基于main集成需重跑CI
> 最后更新：2026-10-07

| 对象 | 实际证据 | 边界 |
| --- | --- | --- |
| r3-ci远程CI | [run 37598715443](https://github.com/lki-H/game-realtime-gm/actions/runs/37598715443)，e286589：Go1.27.1普通测试/vet、V2真实MySQL/Redis race（41.433秒）、双前端ci/build通过 | 不包含Unity/镜像/跨机器 |
| Unity | Unity6000.3.25f1 Windows Player，普通四人、好友双排＋两个单排的任务/重连/逐人结算通过 | 受信事件由Bot提交，不是真战斗模拟 |
| 本机短时soak | Ryzen5 7535H/15.24GiB，302.93秒、101轮、失败0、每轮资产对账0，场景耗时p50/p95/max=1.986/2.083/2.101秒 | 不外推容量上限，未做长时弱网 |
| 镜像 | `game-realtime-gm:r3-local`本机构建通过，约45MB | 无云部署 |
| 监控/GM | 本机采集、故障pending告警与operator可靠重试通过 | 外部通知未接入；综合发布环境另验 |
| 开发day39迁移 | 真实备份SHA256 `3adbdd327285513cb8adc9bab4c8c859058d39f5bd11233f856ee2ebd03b9bbd`；恢复副本和开发库day37-39重复applied，旧2玩家/2资产/0余额，历史/流水摘要不变 | 备份仓库外保留；未回滚schema或改默认玩法 |

`r3-ci`与远程main历史不连通，不能直接创建PR。因此本次 `r3-integration` 从最新远程main建立，精确导入已验证发布文件，不合并不相干的本地历史。保留r3-ci和源工作区的私人/未提交内容。基于main的最终提交和PR合并提交必须各自检查CI。

R4只发布退役/回滚计划；跨机器门槛保持未完成，不切默认V2、不删legacy、不清Redis全库。发布范围见 [运行指南](../pve-release-guide.md)。
