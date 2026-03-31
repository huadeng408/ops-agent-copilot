# 06-工程化能力（第27-29节）

本章目标：从“开发者视角”切换到“交付与运维视角”。

---

## 第27节：测试体系与质量门禁

### 🎯 本节目标
你能设计分层测试体系，并定义发布前必须满足的质量门槛。

### 📚 核心知识点
- 测试分层：单测、集成、评测、冒烟。
- 每层测试目标不同，不能互相替代。
- 质量门禁要量化，不要靠主观判断。

### 🔍 必看代码路径
- [tests](D:/vscode/ops-agent-copilot/tests)
- [scripts/run_eval.py](D:/vscode/ops-agent-copilot/scripts/run_eval.py)
- [scripts/smoke_test.py](D:/vscode/ops-agent-copilot/scripts/smoke_test.py)
- [.vscode/launch.json](D:/vscode/ops-agent-copilot/.vscode/launch.json)

### 🧠 面试高频问题
1. 为什么单测覆盖率高不代表质量高？
2. 你如何设置发布门禁指标？
3. 冒烟测试和压测的作用差别是什么？

### ✅ 标准答案与展开
Q1：单测通常覆盖函数逻辑，无法发现系统集成与数据一致性问题。  
Q2：门禁建议至少含：关键接口通过率、核心评测指标阈值、回归失败数。  
Q3：冒烟验证“功能可用”，压测验证“容量与稳定性”，目标不同不可互代。

### ⚙️ 实战任务
- 增加一组审批状态机异常路径测试。
- 定义一份 release gate 清单（3 条硬性门槛）。

### 🚀 进阶思考
如何引入“风险分级发布门禁”，让高风险改动自动提高验收标准？

---

## 第28节：部署、发布与回滚策略

### 🎯 本节目标
你能给出一套可执行的发布与回滚方案，而不是“有问题就回滚”。

### 📚 核心知识点
- 发布是“代码 + 配置 + 数据库”三件事协同。
- migration 顺序错误是常见生产事故来源。
- 灰度发布和快速回滚是高可用基本能力。

### 🔍 必看代码路径
- [docker-compose.yml](D:/vscode/ops-agent-copilot/docker-compose.yml)
- [start.ps1](D:/vscode/ops-agent-copilot/start.ps1)
- [.env.example](D:/vscode/ops-agent-copilot/.env.example)
- [app/db/migrations](D:/vscode/ops-agent-copilot/app/db/migrations)

### 🧠 面试高频问题
1. 数据库迁移和代码发布谁先谁后？
2. 如何定义“必须回滚”的触发条件？
3. 灰度期看哪些指标最有价值？

### ✅ 标准答案与展开
Q1：优先做向前兼容 migration，再发布新代码；避免新代码依赖未上线 schema。  
Q2：核心业务失败率、P95 突增、审批执行异常率超阈值应触发回滚。  
Q3：看业务成功率、关键状态分布、错误类型结构，不只看 CPU。

### ⚙️ 实战任务
- 写一份“灰度发布 SOP”（步骤、阈值、回滚动作）。
- 设计一条“审批失败率异常”自动回滚规则。

### 🚀 进阶思考
如果是 Go + K8s，如何用金丝雀策略实现自动回滚？

---

## 第29节：SRE 化运营（告警、值班、Runbook）

### 🎯 本节目标
你能把系统从“开发可用”升级成“线上可运维”。

### 📚 核心知识点
- 告警必须有分级和处理流程。
- runbook 是团队经验沉淀工具，不是文档装饰。
- 复盘要输出系统性改进，不做“甩锅报告”。

### 🔍 必看代码路径
- [app/core/observability.py](D:/vscode/ops-agent-copilot/app/core/observability.py)
- [ops/grafana/dashboards/ops-agent-overview.json](D:/vscode/ops-agent-copilot/ops/grafana/dashboards/ops-agent-overview.json)
- [app/api/audit.py](D:/vscode/ops-agent-copilot/app/api/audit.py)
- [docs/STATE_MACHINE_OBSERVABILITY.md](D:/vscode/ops-agent-copilot/docs/STATE_MACHINE_OBSERVABILITY.md)

### 🧠 面试高频问题
1. 如何避免告警疲劳？
2. 故障复盘里最重要的输出是什么？
3. 值班体系如何和研发迭代结合？

### ✅ 标准答案与展开
Q1：减少低价值告警，做聚合与分级，并保证每条告警有明确 owner。  
Q2：最重要是“系统改进项”和“责任闭环”，不仅是时间线记录。  
Q3：把高频事故问题转化为开发 backlog，形成“故障驱动改进”。

### ⚙️ 实战任务
- 输出一份“审批执行失败突增”runbook（定位/止血/复盘）。
- 设计 P1/P2/P3 事件分级标准并给出响应时限。

### 🚀 进阶思考
如何让 runbook 自动化执行（半自动止血），减少人工响应时间？

