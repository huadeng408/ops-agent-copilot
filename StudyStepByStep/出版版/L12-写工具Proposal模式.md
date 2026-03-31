---
title: 写工具Proposal模式
lesson: 12
series: StudyStepByStep 出版版
audience: 后端工程师（Go面试导向）
recommended_time: 90-120分钟
---

# L12 写工具 Proposal 模式

## 本课定位
理解“先描述命令，再执行命令”的治理价值。

## 图解页
```mermaid
flowchart LR
MSG["写请求"] --> PRO["生成Proposal"]
PRO --> AP["审批通过?"]
AP -->|是| EXEC["执行动作"]
AP -->|否| END["拒绝并留痕"]
```

## 术语表
- Proposal：待审批动作
- Command Pattern：命令模式
- Approval Required：需审批状态

## 面试问题与标准答案
1. Proposal模式核心收益？  
答案：把高风险动作前置治理，支持审查、追踪和回放。
2. 为什么payload schema重要？  
答案：它是命令契约，决定校验和兼容演进能力。
3. action_type增多如何维护？  
答案：注册表+统一基类+schema验证，避免分支膨胀。

## 课后任务与参考答案
- 任务：新增一个 `propose_close_ticket`。  
参考：仅生成proposal，不允许直接执行。

## 关键源码锚点
- [app/tools/write_tools.py](../../app/tools/write_tools.py)
- [app/schemas/tool.py](../../app/schemas/tool.py)
- [app/services/agent_service.py](../../app/services/agent_service.py)

## 常见误区
1. 只讲这个功能怎么用，却没有解释为什么这样设计。面试官会继续追问不变量、失败路径和治理边界。
2. 把单机跑通当成生产可用，忽略幂等、并发冲突、审计补偿和可回放。
3. 指标口径与代码实现脱节，只能背结果，不能给出源码证据。

## 实战检查清单
- [ ] 我能用 30 秒说清《写工具Proposal模式》在整条业务链路中的位置。
- [ ] 我能指出至少 3 个源码锚点，并解释每个锚点的职责边界。
- [ ] 我能说出该课对应的核心不变量和一个失败场景。
- [ ] 我准备了当前方案 tradeoff + 下一步优化的双段式回答。
- [ ] 我可以在白板上画出关键调用链，并标注状态变化。

## 60秒面试口播模板
> 如果面试官问到《写工具Proposal模式》，我会先给结论：这部分设计的目标不是功能可用，而是在真实生产约束下可治理、可追责、可演进。
> 第二句我会给代码证据：我会从本课的 3 个源码锚点说明职责分层、数据落点和失败处理路径。
> 第三句我会讲工程取舍：当前方案优先保证一致性和可观测性，同时牺牲了部分开发复杂度。
> 最后我会给优化方向：在不破坏不变量的前提下，说明如何做性能优化或分布式扩展。

## 学习导航
- 对应深度章节：[03-核心模块拆解](../03-核心模块拆解/README.md)
- 对应讲师脚本：[L12-写工具Proposal模式-讲师脚本.md](../讲师版脚本/L12-写工具Proposal模式-讲师脚本.md)
- 建议串联学习：先回看上一课的输入，再用下一课验证当前设计的边界。

## 延伸阅读与参考文献
1. SQLAlchemy 2.0 官方文档
2. Alembic 官方文档（迁移与版本管理）
3. Idempotency-Key 设计实践（Stripe Engineering）
4. Outbox Pattern / Transactional Messaging 实践

## 本课小结
- 已完成本课核心概念、代码路径和面试问答训练。
- 建议在24小时内完成一次口述复盘，巩固可表达能力。

> 页脚：StudyStepByStep 出版版 · L12-写工具Proposal模式 · 最后更新：2026-03-31
