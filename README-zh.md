# NOUS

Intelligence has economic value.

NOUS 是一个去中心化经济系统，将工作量证明从无意义的哈希计算扩展为约束满足问题的求解。节点通过证明推理能力获得经济回报。

## 核心特性

- 三层出块机制：VDF（时间公平）+ CSP（推理门槛）+ PoW（最终竞争）
- 恒定发行：每块 10 NOUS，210 亿总量，约 2000 年发行周期
- 工具中立：AI 模型、传统求解器、混合方法均可参与
- 进化设计：推理难度自动增长 + NP-complete 约束类型预置待激活

## 编译

Requires Go 1.22+.

```bash
go build -o nousd ./cmd/nousd
go build -o nous-cli ./cmd/nous-cli
```

## 文档

- [白皮书](docs/whitepaper.md)

## License

All rights reserved. See LICENSE for details.
