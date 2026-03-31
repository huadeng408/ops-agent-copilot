---
title: Repository抽象与查询性能
lesson: 17
series: StudyStepByStep 出版版
audience: 后端工程师（Go面试导向）
recommended_time: 90-120分钟
---

# L17 Repository 抽象与查询性能

## 本课定位
掌握“抽象可维护性”和“查询性能”之间的平衡。

## 图解页
```mermaid
graph LR
SVC["Service"] --> REPO["Repository方法"]
REPO --> SQL["SQL/ORM查询"]
SQL --> DB["数据库"]
```

## 术语表
- Query Plan：执行计划
- N+1 Query：N+1查询
- Semantic Repository：语义化仓储接口

## 面试问题与标准答案
1. repository会遮蔽性能问题吗？  
答案：会，所以必须结合SQL日志和profiling。
2. 何时直写SQL？  
答案：复杂统计、性能关键路径、ORM表达困难场景。
3. 如何防N+1？  
答案：批量查询、预加载、聚合查询和字段裁剪。

## 课后任务与参考答案
- 任务：抓一条慢SQL并提出优化方案。  
参考：给出索引/改写前后对比。

## 关键源码锚点
- [app/repositories/ticket_repo.py](../../app/repositories/ticket_repo.py)
- [app/repositories/metric_repo.py](../../app/repositories/metric_repo.py)
- [app/repositories/approval_repo.py](../../app/repositories/approval_repo.py)

## 常见误区
1. 只讲这个功能怎么用，却没有解释为什么这样设计。面试官会继续追问不变量、失败路径和治理边界。
2. 把单机跑通当成生产可用，忽略幂等、并发冲突、审计补偿和可回放。
3. 指标口径与代码实现脱节，只能背结果，不能给出源码证据。

## 实战检查清单
- [ ] 我能用 30 秒说清《Repository抽象与查询性能》在整条业务链路中的位置。
- [ ] 我能指出至少 3 个源码锚点，并解释每个锚点的职责边界。
- [ ] 我能说出该课对应的核心不变量和一个失败场景。
- [ ] 我准备了当前方案 tradeoff + 下一步优化的双段式回答。
- [ ] 我可以在白板上画出关键调用链，并标注状态变化。

## 60秒面试口播模板
> 如果面试官问到《Repository抽象与查询性能》，我会先给结论：这部分设计的目标不是功能可用，而是在真实生产约束下可治理、可追责、可演进。
> 第二句我会给代码证据：我会从本课的 3 个源码锚点说明职责分层、数据落点和失败处理路径。
> 第三句我会讲工程取舍：当前方案优先保证一致性和可观测性，同时牺牲了部分开发复杂度。
> 最后我会给优化方向：在不破坏不变量的前提下，说明如何做性能优化或分布式扩展。

## 学习导航
- 对应深度章节：[03-核心模块拆解](../03-核心模块拆解/README.md)
- 对应讲师脚本：[L17-Repository抽象与查询性能-讲师脚本.md](../讲师版脚本/L17-Repository抽象与查询性能-讲师脚本.md)
- 建议串联学习：先回看上一课的输入，再用下一课验证当前设计的边界。

## 延伸阅读与参考文献
1. SQLAlchemy 2.0 官方文档
2. Alembic 官方文档（迁移与版本管理）
3. Idempotency-Key 设计实践（Stripe Engineering）
4. Outbox Pattern / Transactional Messaging 实践

## 本课小结
- 已完成本课核心概念、代码路径和面试问答训练。
- 建议在24小时内完成一次口述复盘，巩固可表达能力。

> 页脚：StudyStepByStep 出版版 · L17-Repository抽象与查询性能 · 最后更新：2026-03-31
