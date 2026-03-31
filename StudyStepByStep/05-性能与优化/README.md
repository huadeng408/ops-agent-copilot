# 05-性能与优化（第23-26节）

本章目标：建立性能治理闭环，而不是“做了优化却说不清收益”。

---

## 第23节：性能基线建立与压测方法

### 🎯 本节目标
学会建立可复现的性能基线，掌握吞吐、延迟、错误率的解释方法。

### 📚 核心知识点
- 基线必须可重复：固定请求集、固定参数、固定环境。
- 关键指标：RPS、P95/P99、错误率、业务状态分布。
- 压测不是跑一个数字，而是对比优化前后变化。

### 🔍 必看代码路径
- [scripts/run_load_test.py](D:/vscode/ops-agent-copilot/scripts/run_load_test.py)
- [eval/benchmarks/20260331](D:/vscode/ops-agent-copilot/eval/benchmarks/20260331)
- [app/core/observability.py](D:/vscode/ops-agent-copilot/app/core/observability.py)

### 🧠 面试高频问题
1. 为什么只看平均耗时不可靠？
2. 如何确定压测瓶颈在服务端还是客户端？
3. 你如何定义“优化有效”？

### ✅ 标准答案与展开
Q1：平均值掩盖尾延迟，真实体验由长尾主导。  
Q2：监控客户端 CPU/网络与服务端指标，对比两端瓶颈信号。  
Q3：优化有效必须同时满足：核心指标改善且无副作用放大。

### ⚙️ 实战任务
- 跑 2 组压测（如 50rps/100rps），对比 P95 和错误率。
- 形成一页压测报告（现象、分析、结论、后续）。

### 🚀 进阶思考
如何设计“持续性能回归”机制，防止版本迭代性能倒退？

---

## 第24节：热路径优化（路由、工具、数据库）

### 🎯 本节目标
学会定位热路径并按收益优先级选择优化点。

### 📚 核心知识点
- 优化顺序：高频路径 > 高耗时路径 > 低收益改动。
- heuristic 命中率高可显著降低 LLM 开销。
- DB 查询优化通常比代码微优化收益更大。

### 🔍 必看代码路径
- [app/agents/router.py](D:/vscode/ops-agent-copilot/app/agents/router.py)
- [app/services/tool_registry.py](D:/vscode/ops-agent-copilot/app/services/tool_registry.py)
- [app/repositories/ticket_repo.py](D:/vscode/ops-agent-copilot/app/repositories/ticket_repo.py)

### 🧠 面试高频问题
1. 你会先优化哪个环节？为什么？
2. heuristic 规则增加会不会带来维护债？
3. 数据库优化最常见误区是什么？

### ✅ 标准答案与展开
Q1：先优化高频且高耗时环节，通常是查询和外部调用。  
Q2：会，因此要做规则分层、冲突检测和回归测试。  
Q3：只加索引不看写放大与查询计划，是典型误区。

### ⚙️ 实战任务
- 对一条慢查询做索引或条件裁剪优化并对比前后耗时。
- 统计 heuristic 命中率并提出提升方案。

### 🚀 进阶思考
你会如何做“热点业务请求缓存”同时保证写后一致性？

---

## 第25节：LLM 成本与稳定性优化

### 🎯 本节目标
掌握模型调用成本控制、失败降级和质量平衡策略。

### 📚 核心知识点
- 不是所有请求都值得调用 LLM。
- 降级策略是生产系统生存线，不是可选项。
- 上下文控制决定 token 成本和延迟。

### 🔍 必看代码路径
- [app/agents/planner_agent.py](D:/vscode/ops-agent-copilot/app/agents/planner_agent.py)
- [app/services/llm_service.py](D:/vscode/ops-agent-copilot/app/services/llm_service.py)
- [app/services/memory_service.py](D:/vscode/ops-agent-copilot/app/services/memory_service.py)
- [app/core/observability.py](D:/vscode/ops-agent-copilot/app/core/observability.py)

### 🧠 面试高频问题
1. 如何减少模型成本而不明显损失质量？
2. 模型异常时为何不能直接失败？
3. 你如何做模型灰度切换？

### ✅ 标准答案与展开
Q1：优先 heuristic 命中，控制上下文长度，限制不必要工具描述。  
Q2：业务系统优先可用，直接失败会放大外部依赖风险。  
Q3：按流量分层灰度，监控准确率、延迟、失败率，达阈值自动回滚。

### ⚙️ 实战任务
- 统计 planner 触发率和平均响应时延。
- 注入 LLM 故障，验证 fallback 是否生效。

### 🚀 进阶思考
若引入多模型路由，如何做“成本上限预算”与“质量下限守护”？

---

## 第26节：可观测驱动优化闭环

### 🎯 本节目标
把优化流程从“感觉优化”变成“数据驱动优化”。

### 📚 核心知识点
- 指标负责发现问题，trace 负责定位问题，日志负责解释细节。
- 业务指标和系统指标要同时看。
- 优化后必须做回归验证，防止局部优化破坏全局。

### 🔍 必看代码路径
- [app/core/observability.py](D:/vscode/ops-agent-copilot/app/core/observability.py)
- [ops/prometheus/prometheus.yml](D:/vscode/ops-agent-copilot/ops/prometheus/prometheus.yml)
- [ops/grafana/dashboards/ops-agent-overview.json](D:/vscode/ops-agent-copilot/ops/grafana/dashboards/ops-agent-overview.json)
- [app/core/logging.py](D:/vscode/ops-agent-copilot/app/core/logging.py)

### 🧠 面试高频问题
1. 你如何定义这个系统的 SLI/SLO？
2. 为什么“只看服务 CPU”不够？
3. 如何避免监控泛滥却没有行动？

### ✅ 标准答案与展开
Q1：SLI 可设为 chat 成功率、P95、审批执行失败率；SLO 是目标阈值。  
Q2：CPU 高不一定影响业务，必须和业务错误率、延迟联合判断。  
Q3：每个监控项都应绑定处理动作（谁值班、何时升级、如何止血）。

### ⚙️ 实战任务
- 设计 3 条关键报警规则并写处理 runbook。
- 用一次压测数据做“问题发现 -> 定位 -> 修复建议”演练。

### 🚀 进阶思考
如何设计“自动异常检测 + 自动回滚建议”的下一代可观测系统？

