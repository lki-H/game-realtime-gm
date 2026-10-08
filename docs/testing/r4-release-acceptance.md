# R4 发布与版本基线

> 文档角色：公开发布范围、验证证据与回滚基线
> 权威级别：L2
> 状态：本机与当前R4发布CI已通过；合并后另核对main Actions
> 最后更新：2026-10-08

## 发布结果

R4默认使用V2，并加入持久化维护准入、operator排空与幂等审计、旧Redis匹配缓存白名单工具。修复角色撤销竞态、旧战绩时区、Run维护饥饿、锁丢失停服和客户端失效恢复。旧一期只显式回归，历史/资产/审计数据保留。

此次发布同时包含此前未合并的R3审计修复；Unity控制面保持已审核脚本，不声称重新完成Editor或跨机器验收。无R5实现、公网部署或新增开发数据迁移。

## 验证层次

发布代码提交 `1d8fc7e96f489ffe2eff0f51a1dc8c05c54ef6e8` 的 [push run 37760910771](https://github.com/lki-H/game-realtime-gm/actions/runs/37760910771) 与 [PR run 37760917194](https://github.com/lki-H/game-realtime-gm/actions/runs/37760917194) 全部通过，Go/backend及两个frontend均成功。PR真实存储race包耗时30.927秒，固定R3/R4二进制已实际构建运行，结束时隔离网络/容器/卷清理成功。发布PR为 [#2](https://github.com/lki-H/game-realtime-gm/pull/2)。后续仅文档补记仍需对应提交的检查通过再合并。

该PR run实际构建服务镜像，Docker image ID为 `sha256:1e4ac5fa35b3f8395b9f69a96235341ec478724d7d42cdcc4e606085c751539e`；这是该次构建的本地镜像身份，未推送镜像仓库，也不是registry manifest digest。部署或重新构建必须重新记录对应身份。

- 源工作区验证：Go全量/vet、真实MySQL/Redis完整V2、Linux race、双前端构建和生产依赖审计；浏览器真实登录/建房/身份失效/localhost通过，见 [复核记录](r4-full-review-20261008.md)。
- 干净发布工作区：从最新main只同步公开清单，重做构建/测试与文件、链接、迁移checksum检查。
- GitHub Actions：Go普通测试/vet、真实MySQL/Redis race、双前端干净构建；CI实际构建并启动当前R4与固定R3二进制，验证HTTP重复事件、Worker重试和资产去重，不以SKIP代替回滚测试。
- 远程检查必须对应当前提交，合并后再查main的实际run；历史R3绿色结果不替代本次检查。

## 回滚基线

固定审核过的R3提交 `64a4263648849586dfad9013d49a664f76181e55` 为兼容回滚版本。CI显式fetch并从该提交编译二进制，不使用浮动分支头。回退时保持V2、同一MySQL schema和现有事实；先排空、关闭准入、停止服务，并在旧R3启动前禁用准入规则，因为R3不读取day40维护开关。流程见 [R4实施](../design/v2-r4-implementation.md)。

| 迁移 | SHA256 |
| --- | --- |
| day37 | `f5445541c250e417217bed2a87bbd5a25b25702ab5b92d857afa7676d963d8bd` |
| day38 | `be1db32047a77d70199aa67ef698148ac74be76aac6e3e2c6afa68e1f281b61d` |
| day39 | `90832beb3615608b13efce331622f33bf75169eecc50b72e435f662946439a71` |
| day40 | `714cef12c0c49184b4b3a0a2c3079bc353eb64d307a0262eb1c695237f9923ab` |

day37—39原字节不变。day40增加两张表；本机开发库已通过备份恢复后迁移，备份SHA256为 `555764a44fbbb8fb6bc1aa3a44b3da1e7d0d3ecae3069589f555f858d166ae59`，备份在仓库外保留。其他环境仍须单独备份、恢复、停写和迁移；不能用旧备份覆盖新资产，不删除day40表或发奖依据。

## 仍未验收

跨机器、共享长期观察/容量、真实战斗服、R5和旧学习包彻底删除继续延期。镜像摘要只有实际构建后才记录，不以commit SHA代替镜像身份。
