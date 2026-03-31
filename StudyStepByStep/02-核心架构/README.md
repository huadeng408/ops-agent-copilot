# 02-核心架构（第05-08节）

本章目标：你要从“模块认知”升级到“架构决策能力”。每一节都围绕“为什么这样设计”展开。

---

## 第05节：分层架构与职责边界

### 🎯 本节目标
学完后你能明确说出每一层应做什么、不应做什么，并识别“职责漂移”。

### 📚 核心知识点（结合源码）
1. **API 层是协议层，不是业务层**  
只做入参解析、依赖注入、错误映射。不要在这里写复杂业务分支。

2. **Service 层是编排层**  
负责业务流程串联、状态推进、事务时机控制。

3. **Repository 层是数据语义层**  
统一 SQL 访问路径，避免 service 到处拼 SQL。

4. **Tool 层是能力封装层**  
把具体能力抽成可组合单元，供路由器和 planner 调用。

### 🔍 必看代码路径（精确到文件/函数）
- [app/api/deps.py](D:/vscode/ops-agent-copilot/app/api/deps.py) `build_tool_registry/get_agent_service`
- [app/services/agent_service.py](D:/vscode/ops-agent-copilot/app/services/agent_service.py) `handle_chat`
- [app/repositories/ticket_repo.py](D:/vscode/ops-agent-copilot/app/repositories/ticket_repo.py)
- [app/tools/base.py](D:/vscode/ops-agent-copilot/app/tools/base.py) `BaseTool`

### 🧠 面试高频问题
1. 你如何判断一段代码应该放在哪一层？
2. 分层越细越好吗？
3. 什么时候 repository 抽象会成为负担？

### ✅ 标准答案与展开
**Q1：如何判断代码层级？**  
标准回答：按“关注点”判断，不按“方便写”判断。  
展开：涉及 HTTP 协议的放 API；涉及流程编排和状态变更放 Service；涉及数据读写细节放 Repository；可复用的业务能力放 Tool。这样后续扩展时不会互相污染。

**Q2：分层越细越好吗？**  
标准回答：不是。分层目标是降低复杂度，不是增加目录数。  
展开：当团队规模小、业务稳定时，过度分层会增加跳转成本。合理策略是“先保持边界清晰，再按复杂度演化层数”。

**Q3：repository 什么时候变负担？**  
标准回答：当它只做机械透传或隐藏关键 SQL 性能问题时。  
展开：如果 repository 只是把 ORM 调用包一层，收益很低。应把它设计成“有业务语义的查询接口”，并保留性能可观测性。

### ⚙️ 实战任务
1. 任选一个接口，画 `API -> Service -> Tool/Repo -> DB` 调用图。
2. 找一段你认为“层次不纯”的代码，给出重构方案。

### 🧪 验收标准
- 能指出至少 2 处潜在职责漂移风险。
- 能解释重构后可维护性如何提升。

### 🚀 进阶思考
Go 中若采用 `internal/api`, `internal/usecase`, `internal/repo`，如何避免接口爆炸？

---

## 第06节：依赖注入与对象生命周期

### 🎯 本节目标
学完后你能解释“请求级对象”与“进程级对象”边界，并避免常见生命周期 bug。

### 📚 核心知识点（结合源码）
1. **请求级依赖**  
`AsyncSession` 和业务 service 绑定在一次请求内，天然隔离。

2. **组装集中化**  
`deps.py` 统一组装，防止每个路由各自 new 造成漂移。

3. **上下文透传**  
`ToolContext` 统一携带 `trace_id/session_id/user`，确保审计一致。

### 🔍 必看代码路径（精确到文件/函数）
- [app/api/deps.py](D:/vscode/ops-agent-copilot/app/api/deps.py) `get_agent_service`
- [app/db/session.py](D:/vscode/ops-agent-copilot/app/db/session.py) `get_db_session`
- [app/tools/base.py](D:/vscode/ops-agent-copilot/app/tools/base.py) `ToolContext`
- [app/services/tool_registry.py](D:/vscode/ops-agent-copilot/app/services/tool_registry.py) `invoke`

### 🧠 面试高频问题
1. 为什么 `AgentService` 不应该做全局单例？
2. 请求级 session 如何防止脏事务污染？
3. 依赖注入会不会影响性能？

### ✅ 标准答案与展开
**Q1：为什么不全局单例？**  
标准回答：因为它绑定请求态对象（session/user/context），单例会导致并发状态污染。  
展开：单例适合无状态组件（配置读取器、常量 registry），不适合持有请求状态的服务对象。

**Q2：如何防止事务污染？**  
标准回答：请求结束时统一 commit/rollback，且 session 不跨请求复用。  
展开：当前 `chat.py` 明确在成功时 commit，异常 rollback，这是最关键的事务边界控制点。

**Q3：注入影响性能吗？**  
标准回答：影响极小，远小于 DB/LLM 调用。  
展开：注入开销通常是对象构造成本，真正瓶颈在 IO。优化应优先针对 DB 查询和外部调用。

### ⚙️ 实战任务
1. 打印请求内 session 对象 id，验证不同请求是否隔离。
2. 在 `deps.py` 增加一个新 service 依赖并完成接线。

### 🧪 验收标准
- 能清晰区分请求级与全局级对象。
- 能解释“为什么事务提交要放在 API 层而非 Service 层”。

### 🚀 进阶思考
Go 使用 `context.Context` 传递请求信息时，如何避免滥用 context 携带业务数据？

---

## 第07节：决策层设计（Heuristic + Planner）

### 🎯 本节目标
学完后你能讲透“混合决策架构”的工程价值，并回答为什么不是纯模型系统。

### 📚 核心知识点（结合源码）
1. **Heuristic 的价值**
- 低成本
- 高确定性
- 可解释

2. **Planner 的价值**
- 覆盖复杂意图
- 通过 function calling 输出结构化工具调用

3. **降级策略**
- planner 失败回退 heuristic，优先保证可用性。

### 🔍 必看代码路径（精确到文件/函数）
- [app/agents/router.py](D:/vscode/ops-agent-copilot/app/agents/router.py) `route`
- [app/agents/planner_agent.py](D:/vscode/ops-agent-copilot/app/agents/planner_agent.py) `plan/_parse_planned_calls`
- [app/agents/assistant_factory.py](D:/vscode/ops-agent-copilot/app/agents/assistant_factory.py) `create_planner`
- [app/services/llm_service.py](D:/vscode/ops-agent-copilot/app/services/llm_service.py) `responses_create`

### 🧠 面试高频问题
1. 为什么不是纯 LLM agent？
2. planner 失败时为什么要降级而不是直接报错？
3. function calling 的参数安全如何保障？

### ✅ 标准答案与展开
**Q1：为什么不是纯 LLM？**  
标准回答：纯 LLM 方案在成本、稳定性、可解释性上都更脆弱。  
展开：高频标准意图用规则路由更稳、更便宜；复杂长尾再调用模型。这样能控制成本并提升线上可预测性。

**Q2：为什么要降级？**  
标准回答：业务系统可用性优先。  
展开：运营系统不能因为外部模型偶发错误就整体不可用。降级策略保证核心能力持续服务，是生产系统最重要的基本盘。

**Q3：参数安全怎么做？**  
标准回答：模型只给建议，执行前必须再过 schema + verifier + 权限校验。  
展开：function calling 输出不是“可信命令”，只能是“候选动作”。最终执行需要多层防线。

### ⚙️ 实战任务
1. 统计 50 条请求中 heuristic 命中率、planner 触发率。
2. 人工模拟 planner 异常，验证系统是否正确回退。

### 🧪 验收标准
- 能给出“何时走 heuristic，何时走 planner”的可量化规则。
- 能复盘一次失败降级案例并给出结论。

### 🚀 进阶思考
如果要做模型 A/B，如何设计“同请求双路决策但单路执行”的安全实验？

---

## 第08节：写操作安全架构（Verifier + 状态机 + 幂等）

### 🎯 本节目标
学完后你能把审批系统讲成“交易系统”而不是“普通审批页面”。

### 📚 核心知识点（结合源码）
1. **Verifier 前置校验**
- 权限、字段合法性、目标资源存在性。

2. **幂等提案**
- 相同业务意图复用提案，避免重复操作与并发脏写。

3. **状态机约束**
- 限定合法迁移路径，防止越权和状态错乱。

4. **并发冲突处理**
- `IntegrityError` / `StaleDataError` 对应不同冲突场景。

### 🔍 必看代码路径（精确到文件/函数）
- [app/services/verifier_service.py](D:/vscode/ops-agent-copilot/app/services/verifier_service.py) `verify_proposal`
- [app/services/approval_service.py](D:/vscode/ops-agent-copilot/app/services/approval_service.py) `create_proposal/approve/reject`
- [app/services/approval_state_machine.py](D:/vscode/ops-agent-copilot/app/services/approval_state_machine.py) `ensure_transition`
- [app/repositories/approval_repo.py](D:/vscode/ops-agent-copilot/app/repositories/approval_repo.py)

### 🧠 面试高频问题
1. 幂等键为什么这么设计？
2. 状态机为什么独立成模块？
3. 执行失败为什么不自动重试？

### ✅ 标准答案与展开
**Q1：幂等键设计逻辑？**  
标准回答：以业务意图为核心而非请求 ID。  
展开：请求 ID 每次都变，无法识别重复意图。幂等键应由“会话、动作类型、目标对象、payload”等业务维度构成，才能正确复用。

**Q2：为什么状态机独立？**  
标准回答：把规则从流程代码中抽离，降低错误率并提升可测试性。  
展开：如果状态校验散落在多个 if/else 中，后期增加新状态极易出错。独立模块可集中维护迁移矩阵和测试用例。

**Q3：失败为何不自动重试？**  
标准回答：写操作重试可能放大副作用，必须先定位失败类型。  
展开：例如“目标不存在”属于业务错误，重试无意义；“短暂锁冲突”才可能重试。盲目重试会把问题扩大成事故。

### ⚙️ 实战任务
1. 重复提交同一 proposal，验证复用行为。
2. 并发 approve 同一审批单，观察冲突处理结果。
3. 记录一次 `execution_failed` 的完整审计链。

### 🧪 验收标准
- 能复述完整状态流转图。
- 能区分“幂等冲突”和“并发更新冲突”。

### 🚀 进阶思考
如果要做跨服务执行（审批服务和工单服务分离），如何用 Outbox + 幂等消费保证最终一致？

