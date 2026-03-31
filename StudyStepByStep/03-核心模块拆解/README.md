# 03-核心模块拆解（第09-18节）

本章是整套课程最核心的部分。建议至少投入 3-5 天，逐节跑代码、做实验、做复盘。

---

## 第09节：AgentService 编排核心

### 🎯 本节目标
掌握 `handle_chat` 的全流程，能够在面试中画出时序图并解释每一步价值。

### 📚 核心知识点（结合源码）
- 编排入口统一承接会话写入、审计、路由、工具执行和响应组装。
- 编排层是业务“真核心”，接口层只是传输外壳。
- `status=completed` 与 `status=approval_required` 两种终态决定后续流程。

### 🔍 必看代码路径
- [app/services/agent_service.py](D:/vscode/ops-agent-copilot/app/services/agent_service.py) `handle_chat`
- [app/api/chat.py](D:/vscode/ops-agent-copilot/app/api/chat.py) `chat`
- [app/repositories/session_repo.py](D:/vscode/ops-agent-copilot/app/repositories/session_repo.py)

### 🧠 面试高频问题
1. 为什么编排要集中在 `AgentService`？
2. 这个函数偏长，你如何控制复杂度？
3. 事务边界为什么在 API 层提交？

### ✅ 标准答案与展开
Q1：集中编排能避免流程碎片化，便于统一治理、审计和观测。  
Q2：通过“按阶段拆 helper + 明确状态机分支 + 结构化日志”控制复杂度，而不是盲目拆文件。  
Q3：API 层持有请求级 session，上层最清楚请求成败；service 中间提交会引入中间态风险。

### ⚙️ 实战任务
- 给 `handle_chat` 增加阶段耗时日志（接收/路由/工具/回包）。
- 画完整时序图并和日志对齐。

### 🚀 进阶思考
Go 中会否把编排拆成 pipeline？如何避免过度抽象导致排障困难？

---

## 第10节：ToolRegistry 设计与执行治理

### 🎯 本节目标
理解统一工具治理的价值，能回答“为什么不用 if/else 直接调工具”。

### 📚 核心知识点
- registry 管注册、查找、执行、埋点、审计。
- 工具执行返回结构化 `ToolExecutionRecord`，上层无需关心具体工具细节。
- 统一异常路径是生产可维护性的关键。

### 🔍 必看代码路径
- [app/services/tool_registry.py](D:/vscode/ops-agent-copilot/app/services/tool_registry.py) `invoke`
- [app/schemas/tool.py](D:/vscode/ops-agent-copilot/app/schemas/tool.py)
- [app/services/audit_service.py](D:/vscode/ops-agent-copilot/app/services/audit_service.py)

### 🧠 面试高频问题
1. registry 真正解决了什么问题？
2. 为什么埋点放 registry 而不是每个工具里？
3. 工具超时/熔断应该放哪层？

### ✅ 标准答案与展开
Q1：解决扩展性和治理一致性，避免工具逻辑散落。  
Q2：埋点统一可保证口径一致，避免每个工具各自定义指标导致不可比。  
Q3：超时通常在 registry 或更外层执行器实现；熔断在 client 侧更常见，视工具依赖而定。

### ⚙️ 实战任务
- 新增一个只读工具并在 registry 注册。
- 给 `invoke` 添加超时保护（例如 asyncio.wait_for）。

### 🚀 进阶思考
工具数上百后如何分域注册（按业务域分 registry）？

---

## 第11节：只读工具与 SQL 安全防线

### 🎯 本节目标
学会把“数据查询能力”做成可控能力，而不是任意 SQL 的后门。

### 📚 核心知识点
- 只读工具封装业务查询，避免前端直接拼 SQL。
- `SQLGuard` 防注入、防危险语句、防越界查询。
- 结果规模控制是稳定性保护，不只是性能优化。

### 🔍 必看代码路径
- [app/tools/readonly_tools.py](D:/vscode/ops-agent-copilot/app/tools/readonly_tools.py)
- [app/tools/sql_guard.py](D:/vscode/ops-agent-copilot/app/tools/sql_guard.py) `validate`
- [app/services/verifier_service.py](D:/vscode/ops-agent-copilot/app/services/verifier_service.py) `verify_sql/verify_result_size`

### 🧠 面试高频问题
1. SQLGuard 能挡住哪些风险？
2. 为什么只读查询也要限制返回量？
3. 你如何做查询白名单/黑名单治理？

### ✅ 标准答案与展开
Q1：可阻止明显危险语句与非法模式，但不能替代最小权限和参数化查询。  
Q2：大结果会拖垮 API、网络和前端渲染，且常常没有业务价值。  
Q3：优先白名单，黑名单只作补充；高风险查询必须有审批或管理员权限。

### ⚙️ 实战任务
- 构造 5 条合法/非法 SQL 进行验证。
- 将结果上限改成配置项并验证。

### 🚀 进阶思考
Go 中如何结合 `sqlc` 与策略引擎实现“静态安全 + 运行时限制”？

---

## 第12节：写工具（Proposal Tool）模式

### 🎯 本节目标
掌握“命令描述先行、执行后置”的设计思想。

### 📚 核心知识点
- 写工具只负责产 proposal，不直接写库。
- proposal 是可审查、可追踪、可重放的命令描述。
- `requires_approval=True` 是流程分叉关键位。

### 🔍 必看代码路径
- [app/tools/write_tools.py](D:/vscode/ops-agent-copilot/app/tools/write_tools.py)
- [app/services/agent_service.py](D:/vscode/ops-agent-copilot/app/services/agent_service.py) proposal 分支

### 🧠 面试高频问题
1. proposal 模式相比直接执行的优势？
2. payload schema 为什么重要？
3. action_type 增长后如何保持可维护？

### ✅ 标准答案与展开
Q1：把高风险动作从“即时执行”变成“可审查流程”，显著降低误操作。  
Q2：schema 是命令契约，缺它就无法做版本演进与校验。  
Q3：使用统一基类 + action 注册表 + schema 校验，避免 if/else 爆炸。

### ⚙️ 实战任务
- 新增一个 `propose_close_ticket` 工具（仅提案）。
- 写该 action 的 payload 约束说明。

### 🚀 进阶思考
若未来引入审批模板系统，proposal 如何支持模板参数化？

---

## 第13节：ApprovalService 深拆（一致性与并发）

### 🎯 本节目标
把审批服务讲成“交易系统”，具备并发与幂等思维。

### 📚 核心知识点
- 幂等键复用与重试键策略。
- 批准后执行、失败回写、状态推进顺序。
- 并发冲突处理：`IntegrityError` 与 `StaleDataError`。

### 🔍 必看代码路径
- [app/services/approval_service.py](D:/vscode/ops-agent-copilot/app/services/approval_service.py) `create_proposal/approve/reject`
- [app/services/approval_state_machine.py](D:/vscode/ops-agent-copilot/app/services/approval_state_machine.py)

### 🧠 面试高频问题
1. 如何证明审批不会重复执行？
2. 为什么先改 `approved` 再执行动作？
3. 执行失败后如何恢复？

### ✅ 标准答案与展开
Q1：幂等键去重 + 状态机约束 + 并发冲突处理三层防线共同保证。  
Q2：先落审批状态可避免“执行成功但状态未知”的幽灵状态。  
Q3：失败落 `execution_failed`，保留错误信息与版本，后续走人工复核或重提案流程。

### ⚙️ 实战任务
- 并发提交相同 proposal，验证是否复用。
- 并发 approve 同审批单，验证冲突返回。

### 🚀 进阶思考
如果执行动作需要调用外部系统，如何设计补偿事务？

---

## 第14节：审计系统与可追踪性设计

### 🎯 本节目标
掌握“可回放系统”思维，能够按 trace_id 复盘业务事实。

### 📚 核心知识点
- 审计日志是结构化业务事件，不是普通文本日志。
- 事件类型分层：chat、tool、proposal、approval、execution。
- 审计和工具调用日志分表有利于查询与存储策略。

### 🔍 必看代码路径
- [app/services/audit_service.py](D:/vscode/ops-agent-copilot/app/services/audit_service.py)
- [app/repositories/audit_repo.py](D:/vscode/ops-agent-copilot/app/repositories/audit_repo.py)
- [app/api/audit.py](D:/vscode/ops-agent-copilot/app/api/audit.py)

### 🧠 面试高频问题
1. 为什么要事件化审计？
2. trace_id 与审计的关系是什么？
3. 审计数据膨胀如何治理？

### ✅ 标准答案与展开
Q1：事件化审计可做回放、统计、告警和合规，不依赖日志解析。  
Q2：trace_id 是跨模块关联键，能将一次请求的所有动作串起来。  
Q3：冷热分层存储、字段裁剪、归档策略和 TTL 是常见治理手段。

### ⚙️ 实战任务
- 用 trace_id 完整回放一次审批流程。
- 新增一个 `event_type` 过滤条件并验证。

### 🚀 进阶思考
审计若迁移到 ES/ClickHouse，如何保证写入可靠性与查询一致性？

---

## 第15节：MemoryService 与会话上下文

### 🎯 本节目标
理解多轮会话上下文管理，掌握成本与效果的平衡点。

### 📚 核心知识点
- 会话历史不是无限拼接，需截断与摘要。
- summary + recent messages 的组合是性价比较高方案。
- 上下文质量直接影响工具决策准确性。

### 🔍 必看代码路径
- [app/services/memory_service.py](D:/vscode/ops-agent-copilot/app/services/memory_service.py)
- [app/repositories/session_repo.py](D:/vscode/ops-agent-copilot/app/repositories/session_repo.py)
- [app/agents/planner_agent.py](D:/vscode/ops-agent-copilot/app/agents/planner_agent.py) `_build_input`

### 🧠 面试高频问题
1. 为什么不用“全量历史”？
2. summary 出错会有什么后果？
3. memory 与 RAG 有何边界？

### ✅ 标准答案与展开
Q1：全量历史成本高、噪声大、响应慢，不适合生产。  
Q2：summary 偏差会把决策带偏，需定期重算和人工抽检。  
Q3：memory 解决会话内语境，RAG 解决外部知识检索，二者互补。

### ⚙️ 实战任务
- 调整 `keep_recent_message_count` 并评估回答变化。
- 打印 planner 输入，验证上下文注入内容。

### 🚀 进阶思考
如何给 summary 增加“置信度”和“更新时间”，提升可解释性？

---

## 第16节：ReportService 与聚合能力

### 🎯 本节目标
掌握跨仓储聚合查询模式，能够解释“日报能力”的工程实现。

### 📚 核心知识点
- 报表本质是多数据源聚合，而非单 SQL 查询。
- 报告工具是“业务价值密度高”的复合工具。
- 默认回退到 report 是一种保底用户体验设计。

### 🔍 必看代码路径
- [app/services/report_service.py](D:/vscode/ops-agent-copilot/app/services/report_service.py)
- [app/services/agent_service.py](D:/vscode/ops-agent-copilot/app/services/agent_service.py) `generate_report` 分支

### 🧠 面试高频问题
1. 为什么日报做成工具而非独立接口？
2. 聚合查询慢怎么办？
3. 报表口径变化如何治理？

### ✅ 标准答案与展开
Q1：做成工具便于与对话系统统一编排，复用决策和审计链路。  
Q2：可采用预计算、缓存、异步生成与分层查询策略。  
Q3：口径版本化，输出中携带口径版本号并保留历史定义。

### ⚙️ 实战任务
- 给日报增加“审批执行成功率”字段。
- 输出一份报表生成性能剖析（阶段耗时）。

### 🚀 进阶思考
如何把日报生成改为异步任务并提供结果回查接口？

---

## 第17节：Repository 设计与查询性能基础

### 🎯 本节目标
理解 repository 抽象价值，能够把“数据访问”讲出工程权衡。

### 📚 核心知识点
- repository 应表达业务语义，不是 ORM 透传。
- 慢查询优化应结合索引、过滤条件、返回字段裁剪。
- “可维护性”与“极致性能”通常存在张力。

### 🔍 必看代码路径
- [app/repositories/ticket_repo.py](D:/vscode/ops-agent-copilot/app/repositories/ticket_repo.py)
- [app/repositories/metric_repo.py](D:/vscode/ops-agent-copilot/app/repositories/metric_repo.py)
- [app/repositories/approval_repo.py](D:/vscode/ops-agent-copilot/app/repositories/approval_repo.py)

### 🧠 面试高频问题
1. repository 会不会掩盖 SQL 问题？
2. 什么时候该直接手写 SQL？
3. 如何避免 N+1 查询？

### ✅ 标准答案与展开
Q1：会，所以必须结合 SQL 日志和 profiling。  
Q2：复杂统计、性能敏感路径和 ORM 难表达场景优先手写 SQL。  
Q3：批量查询、预加载、聚合查询与字段裁剪是常见策略。

### ⚙️ 实战任务
- 开启 SQL 输出，抓取一次 chat 的 SQL 序列。
- 选一条查询给出索引优化方案。

### 🚀 进阶思考
Go 中 `sqlc` 与 `gorm` 混用时如何做规范，避免团队风格失控？

---

## 第18节：迁移、灌数与管理面板

### 🎯 本节目标
掌握从“空仓库”到“可演示系统”的工程闭环。

### 📚 核心知识点
- migration 保障 schema 演进可追踪。
- demo seed 保证演示与回归可复现。
- admin 面板显著降低联调成本，提升非研发协作效率。

### 🔍 必看代码路径
- [app/db/migrations/env.py](D:/vscode/ops-agent-copilot/app/db/migrations/env.py)
- [scripts/init_demo_data.py](D:/vscode/ops-agent-copilot/scripts/init_demo_data.py) `main`
- [app/api/admin.py](D:/vscode/ops-agent-copilot/app/api/admin.py)
- [docker-compose.yml](D:/vscode/ops-agent-copilot/docker-compose.yml)

### 🧠 面试高频问题
1. 为什么 demo seed 是工程能力而不是“演示脚本”？
2. 迁移脚本如何避免破坏线上？
3. 管理后台为何值得做？

### ✅ 标准答案与展开
Q1：它让功能验证、回归、面试演示具备可复现性，减少“环境运气”。  
Q2：采用向前兼容迁移、灰度验证、回滚预案和数据备份策略。  
Q3：管理后台把排障和验收成本从“命令行专家”降到“团队共用能力”。

### ⚙️ 实战任务
- 从空库执行迁移+灌数，验证系统可用。
- 在 admin 完成一次审批并核对数据变化。

### 🚀 进阶思考
如果要发布到企业内网，admin 页面如何做权限、审计和防误操作二次确认？

