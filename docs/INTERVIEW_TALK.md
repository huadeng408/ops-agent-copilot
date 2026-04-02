# Interview Talk

## 3-minute version

这个项目我现在会把它定义成一个面向企业运营场景的 Copilot / Agent 后端，而不是一个单纯“接了大模型”的聊天应用。

在线主服务已经切到 Go，Go 负责请求编排、工具执行、审批流、审计追踪、缓存、指标和 tracing 这些主链路；Python 只保留离线评测和压测脚本。这样拆分的原因很直接: 在线链路更强调吞吐、可观测、可控性和工程治理，Go 更适合做这个角色；而离线评测和批量实验更适合继续用 Python 快速迭代。

请求进入 `POST /api/v1/chat` 之后，系统先生成 `trace_id`，加载会话记忆，再由 planner 决定走哪类工具。这里我做的是“LLM 只负责规划，执行必须走确定性链路”的架构。只读请求可以直接走查询工具，比如退款率指标、超 SLA 工单、工单详情、发布记录、日报生成和异常归因。所有写操作都不会直接由模型落库，而是统一抽象成 proposal，进入审批流。

审批流的状态机是 `pending -> approved -> executed` 或 `pending -> rejected`。我在这里做了三个关键约束:

- 重复 proposal 通过 `idempotency_key` 收敛。
- 审批过程通过 `version` 做乐观锁控制并发。
- 真正写业务数据时，`ticket_actions.approval_id` 唯一约束防止重复执行。

安全上我没有把希望寄托在“模型足够聪明”，而是通过架构收口。自由查询只允许白名单只读 SQL，并且要经过 `SQLGuard` 和 `Verifier`；写操作只能通过参数化业务执行器，而且必须先审批。这样模型最多只能给出建议，不能直接改业务系统。

这个项目我认为最有价值的点，不是做了多少 Agent 技巧，而是把 Agent 放进了一个真实后端治理框架里，补齐了审批、安全、审计、幂等、可观测和压测这些企业场景真正会问的东西。

## Interviewer-facing positioning

如果面试官只看简历、不看代码，我建议你统一成下面这套口径:

- 这是一个 Go 主导的企业运营 Agent 后端。
- 架构关键词是 “LLM planning + deterministic execution + approval-gated writes”。
- 亮点不是 multi-agent，而是治理闭环和工程落地。
- Python 不是主服务，只是评测和压测工具层。

## Five common follow-ups

### 1. Why not multi-agent first?

因为这个场景的核心矛盾不是 Agent 数量，而是高风险写操作如何治理。先把查询、分析、proposal、审批、审计这条闭环打稳，比上来拆多个 agent 更符合真实业务优先级，也更容易把边界讲清楚。

### 2. Why keep LLM at the planning layer only?

因为企业内部系统最怕的是不可控写入。模型擅长理解自然语言和做意图规划，但不适合直接持有数据库写权限。所以我把模型限制在 “决定调用什么工具、生成什么 proposal” 这一层，真正执行必须走确定性工具和审批状态机。

### 3. What proves it is not just a demo?

我会从三个点回答:

- 有真实 API 主链路，不是 notebook。
- 有审批状态机、幂等和乐观锁，不是一次性脚本。
- 有 metrics、audit、trace 和压测产物，可以追踪性能和风险。

### 4. What is the biggest current bottleneck?

截至 2026-04-02，真正的瓶颈不是数据库，而是同步远程 LLM 规划。启用远程模型时，吞吐会明显受上游模型 RT 和并发限制影响，所以当前更稳的线上口径应该是: 远程 LLM 适合作为可选规划路径，主链路演示和基线吞吐以 heuristic / deterministic path 为主。

### 5. Why is Go the main service now?

因为这个项目后面更像一个治理型后端，而不是原型期的 AI demo。在线链路里最重要的是请求编排、缓存、审计、审批、指标、trace 和稳定性，这部分 Go 的类型约束、部署形态和运行时成本更合适。Python 继续留在评测、压测和实验层，反而边界更清楚。

## 90-second demo script

### Startup

```powershell
.\start.ps1 -SkipLLMCheck
```

### Run the closed-loop demo

```powershell
.\scripts\interview_demo.ps1
```

### What to say while it runs

1. 先演示只读查询，说明自然语言请求已经能被稳定路由到确定性工具。
2. 再演示写请求只会生成 proposal，不会直接改工单。
3. 然后 approve proposal，展示写操作经过审批后才执行。
4. 最后查 audit 和 ticket detail，证明系统能把“请求理解 -> proposal -> 审批执行 -> 审计追踪”串起来。

## Resume-safe phrasing

如果面试官顺着简历深挖，建议你坚持下面这句话，不要临时拔高:

> 构建 Go 主导的企业运营 Copilot / Agent 后端，采用 LLM 规划加确定性执行架构，将高风险写操作统一收敛为 proposal 并进入审批流，结合幂等、乐观锁、SQL Guard、审计和可观测能力，完成查询、分析、审批执行到审计追踪的闭环。

## Do not overclaim

下面这些说法现在不建议硬讲:

- “已经稳定扛住 100/200 RPS 的真实远程 LLM 在线流量”
- “这是企业级多系统联动生产系统”
- “LLM planner 已经是主要高吞吐主链路”

更稳妥的讲法是:

- 单机 deterministic path 有真实压测和 demo 闭环。
- 同步远程 LLM 模式已经做过验证，但当前明确识别为性能瓶颈。
- 项目重点是治理和落地能力，而不是盲目追求 Agent 自主性。
