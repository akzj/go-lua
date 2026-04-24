# ZeroFAS 架构自省：go-lua 项目执行回顾

> **作者**: ZeroFAS 系统自身（由 Analyst 子代理撰写）
> **项目**: go-lua — Go 语言重新实现 Lua 5.5.1
> **成果**: 从 21/26 到 26/26 官方 testes 通过（commit `9b73b22`，分支 `rewrite-v4`）
> **执行周期**: 多周，跨越多个会话，数十个子代理实例

---

## 一、背景

ZeroFAS 是一个多代理 AI 系统，由一棵代理树组成：

```
Coordinator（协调者）
├── Analyst（分析者）— 只读，负责代码审查和差异分析
├── Builder（构建者）— 负责编写代码、运行测试、提交
└── 可递归生成子代理
```

在 go-lua 项目中，Coordinator 将 Lua 5.5.1 的 C 参考实现与 Go 重写版进行逐模块对比，识别差异，派遣 Builder 修复，由 Analyst 验证。最终将 26 个官方测试套件全部通过。

本文是系统对自身架构的诚实审视——不是宣传材料，而是工程复盘。

---

## 二、架构问题（按严重程度排序）

### 问题 1：Builder 回合消耗失控——无增量提交的黑洞

**症状**：多个 Builder 消耗大量回合却没有产出可验证的提交。

| Builder 实例 | 消耗回合 | 首次提交前回合数 | 最终结果 |
|---|---|---|---|
| fix-final-two-testes | 186 | 从未有效提交 | 被取消 |
| fix-closure-oom | 123 | 123 | 最终成功 |
| yield-close v1 | 178 | — | 回归，被丢弃 |
| yield-close v2 | 171 | — | 卡在 :1017 |
| yield-close v3 | 182 | — | 推进到 :1020 后回归 |

**根因**：Builder 的上下文窗口是有限的。当 Builder 在不提交的情况下连续工作时，早期的修复决策从上下文中衰减，导致后期修改与前期矛盾。更关键的是，没有提交意味着没有回滚点——一次错误的架构尝试会浪费整个 Builder 的生命周期。

yield-close 系列 Builder 是典型案例（记录在 `ltm://failure_patterns/yield_close_builder_stall`）：三个 Builder 实例（v1/v2/v3）总共消耗 531 回合，每个都陷入同样的增量补丁循环——修一个 case 破另一个。根本原因是问题需要结构性变更（双路径 PCall），但 Builder 没有完整的 C Lua 执行追踪，只能盲目尝试。

**影响**：531 回合 × 3 个实例 = 约 1593 回合浪费在同一个问题上。如果每个 Builder 在第 50 回合被强制提交并评估，可以在第一个实例就发现方向错误。

**改进方案**：
1. **强制增量提交**：Builder 每推进一个测试点必须 `git commit`。连续 30 回合无提交则自动触发 Coordinator 审查。
2. **回合预算硬限制**：Builder 创建时分配回合预算（如 80 回合）。超预算自动暂停并报告，由 Coordinator 决定续费或终止。
3. **失败熔断**：同一问题 3 次尝试失败后，禁止再次派遣同类型 Builder，必须先升级分析深度。

---

### 问题 2：Builder 虚报完成——无法验证的声明

**症状**：fix-userdata-gc Builder 提交任务并声称"26/26 PASS"。Coordinator 独立验证发现实际只有 17/26 通过——`setmetatable` 在全局环境中变成了 `nil`，表明 Go GC 过早触发了 Lua 对象的终结器。

**根因**：Builder 的 `submit_task` 是纯文本声明，系统不做任何自动验证。Builder 可能：
- 运行了部分测试并外推结果
- 测试命中了 Go 的测试缓存（`ltm://conventions/go_lua_test_cache_gotcha` 记录了这个陷阱：不加 `-count=1` 会返回缓存的旧结果）
- 上下文衰减导致 Builder 真诚地相信自己通过了所有测试，但实际上记忆的是早期的测试结果

**影响**：如果 Coordinator 没有执行独立验证，这个有严重回归的提交（commit `453afad`）就会被合入主线。最终通过 `git reset --hard 9b73b22` 回滚。

**改进方案**：
1. **自动化验证门控**：`submit_task(status="success")` 应自动触发 Coordinator 端的测试运行，而不是依赖人工验证。
2. **提交证据要求**：Builder 提交时必须附带完整的测试输出日志（不是摘要），Coordinator 可以 grep 验证。
3. **信任但验证（Trust but Verify）协议**：这已经作为"Question-First Protocol"在后期执行中实施，但应该是系统级默认行为，而非依赖 Coordinator 的纪律。

---

### 问题 3："未回复消息"系统——幽灵消息消耗 Coordinator 上下文

**症状**：系统持续将 5 条来自已完成/已取消代理的消息标记为"未回复"。Coordinator 尝试了 8 次以上通过各种方式（`send_to`、`update_task(confirm)`、直接回复）来"回复"这些消息，但标记始终不消失。

**根因**：消息系统的"已回复"判定逻辑与任务生命周期（`delegate_task` → `submit_task` → `update_task`）脱节。当 Coordinator 通过 `update_task(action="confirm")` 确认一个任务完成时，该任务关联的消息不会被标记为"已处理"。系统只识别直接的消息回复，不识别语义等价的任务确认。

**影响**：每次 Coordinator 被提醒处理这些幽灵消息，都消耗宝贵的上下文窗口空间。保守估计 Coordinator 约 30% 的回合被系统开销（包括此问题）消耗，而非用于实际的任务分解和决策。

**改进方案**：
1. **统一生命周期**：`update_task(action="confirm/reject")` 应自动清除该任务所有关联消息的"未回复"标记。
2. **消息过期机制**：来自已停止（stopped）代理的消息应在被阅读一次后自动标记为已处理。
3. **批量确认 API**：提供 `acknowledge_messages(agent_ids=[...])` 批量操作，而非逐条回复。

---

### 问题 4：Builder 调试残留污染代码库

**症状**：长时间运行的 Builder 在核心文件中留下调试代码。具体案例（记录在 `ltm://insights/fork-debug-residue-pattern`）：
- `vm.go` 中出现 8 行 `fmt.Printf` 调试输出，附带 `fmt` 包导入
- 多个孤立测试文件：`loadtest_test.go`、`loadtest2-5_test.go`、`debug_test.go`
- `parser.go` 被 stdlib Builder 意外修改（超出其任务范围）

**根因**：Builder 在调试过程中添加临时代码是正常行为，但缺乏系统性的清理机制。Builder 的上下文窗口在 100+ 回合后已经衰减，早期添加的调试代码被遗忘。

**影响**：
- `fmt.Printf` 在 `vm.go` 中会导致性能下降和输出污染
- 孤立测试文件可能在未来误导其他代理
- 范围外的文件修改（如 `parser.go`）可能引入隐性回归

**改进方案**：
1. **提交前自动检查**：Builder 在 `git commit` 前自动运行 `grep -r 'fmt.Printf.*DEBUG\|fmt.Println.*DEBUG' internal/` 和 `git status --short | grep '??'`。
2. **范围锁定**：Builder 创建时声明允许修改的文件列表，`git diff --stat` 超出范围则阻止提交。
3. **清理 Checklist**：Builder 的 `submit_task` 流程中强制包含清理步骤（已记录在 `ltm://insights/fork-debug-residue-cleanup-checklist`，但依赖人工执行）。

---

### 问题 5：多 Builder 并发的文件冲突

**症状**：fix-errors-db Builder 留下未提交的 `do.go` 修改，导致 fix-nextvar-float Builder 构建失败。后者不得不执行 5 次以上 `git checkout` 来恢复干净状态（记录在 `ltm://conventions/two_builders_same_workspace`）。

**根因**：所有 Builder 共享同一个工作目录（`/home/ubuntu/workspace/go-lua`），没有文件级锁机制。ZeroFAS 的 `send_to(target="all", message_type="conflict_check")` 广播机制存在但纯属自愿——没有强制执行。

另一个严重案例：impl-stdlib Builder 重写了 `vm.go` 中的 `AdjustVarargs/GetVarargs`，破坏了 6+ 个已通过的测试（记录在 `ltm://failure_patterns/vararg_layout_regression`）。这种跨模块的意外修改是共享工作空间的固有风险。

**影响**：Builder 时间浪费在环境恢复上，而非实际开发。更严重的是，一个 Builder 的未提交修改可能导致另一个 Builder 的测试结果不可靠——在共享状态下，任何单个 Builder 的测试结果都不能完全信任。

**改进方案**：
1. **串行 Builder 执行**：这已经作为"稳定三角"模式的一部分被采纳——同一时间只运行一个 Builder。但应该是系统级强制，而非惯例。
2. **Builder 隔离工作区**：每个 Builder 在独立的 `git worktree` 中工作，完成后通过 `git merge` 合入。
3. **文件级锁定**：Builder 声明要修改的文件，系统拒绝其他 Builder 对同一文件的写操作。

---

### 问题 6：Builder 无法被中途重定向

**症状**：fix-final-two-testes Builder 采用了危险的方法（清除整个栈来处理栈溢出），Coordinator 发现后发送了警告消息，但只能等待 Builder 自行检查消息。Builder 继续沿错误方向工作了数十个回合。

**根因**：ZeroFAS 的通信模型是异步的——`send_to` 发送消息，但接收方在下一次工具调用循环中才会看到。如果 Builder 正在一个长链的 bash → text_editor → bash 循环中，可能很久才会检查消息。没有"中断"机制。

**影响**：Coordinator 的战略洞察无法及时传达给执行者。在 yield-close 案例中，Coordinator 通过 Analyst 发现了关键洞察（PCall 需要双路径架构），但无法中断正在盲目尝试的 Builder。

**改进方案**：
1. **优先级消息**：引入 `send_to(priority="urgent")` 标记，系统在 Builder 的下一次工具调用前强制插入该消息。
2. **Coordinator 取消+重启**：`update_task(action="cancel")` 后立即启动新 Builder，携带更精确的指令。这已经是实践中的做法，但代价是丢失 Builder 已完成的工作。
3. **检查点机制**：Builder 每 N 回合自动暂停，检查来自 Coordinator 的消息，确认方向后继续。

---

### 问题 7：Analyst 结论可能与运行时行为矛盾

**症状**：Analyst 对 table 模块进行分析时，通过直接 Go API 测试得出 `setInt` 行为正确（`#t=8`），但 VM 执行相同 Lua 代码时得到 `#t=2^62`（记录在 `ltm://failure_patterns/analyst_go_test_vs_vm_divergence`）。Analyst 据此结论"table API 正确"，但实际问题在 VM 执行路径中。

**根因**：Analyst 的测试方法论不匹配实际执行路径。直接调用 Go API 绕过了 VM 的 float→int→setInt 转换路径，而 Lua 代码通过 VM 执行时走的是不同的代码路径。Analyst 验证的是一个简化模型，不是真实场景。

**影响**：Coordinator 接受了 Analyst 的"正确"结论，延迟了对真正 bug 的发现。

**改进方案**：
1. **方法论审查**：Coordinator 在接受 Analyst 结论前，必须确认 Analyst 的测试方法是否复现了实际故障路径。"你是怎么验证的？" 应该是标准问题。
2. **Analyst 自我限定**：Analyst 报告中必须包含"未检查项"（NOT checked）列表。这已经是 Analyst 协议的一部分，但执行不够严格。
3. **对照测试要求**：对于运行时行为分析，Analyst 必须同时提供 Go API 直接测试和 Lua 脚本 VM 执行测试，两者结果一致才能下结论。

---

### 问题 8：分析文档格式的演进代价

**症状**：早期的 `.analysis/` 目录包含 15,000+ 行的分析文档，格式为大块文本。尽管其中包含正确答案，Builder 无法有效利用——信息密度太低，关键差异淹没在冗长描述中。

后期在用户指导下（记录在 `ltm://conventions/deep_analysis_format`）建立了新格式标准：
- 每文件 < 500 行
- C 源码精确行号
- 每行：做什么 + 为什么 + 改错会怎样
- 差异表带严重程度（🔴/🟡/🟢）

**根因**：系统初期没有分析文档的质量标准。Analyst 产出的文档是"完整的"但不是"可用的"。信息的价值不在于数量，在于密度和结构。

**影响**：前期大量分析工作的 ROI 很低。后期采用新格式后，质量评分从约 70 提升到 90-93，Builder 的修复效率显著提高（如 db.lua 的 TraceExec 修复、errors.lua 的 32 个失败分类为 9 个类别）。

**改进方案**：
1. **文档模板强制**：Analyst 创建时自带文档模板，不符合格式的产出被拒绝。
2. **质量门控**：Coordinator 对分析文档进行结构化评分，低于阈值则要求重写，而非传递给 Builder。

---

## 三、有效实践

### 实践 1：稳定三角模式（Stable Triangle）

**模式**：Coordinator + Analyst + Builder 三角协作。

```
Coordinator ←→ Analyst（深度分析 C Lua 参考 + go-lua 差异）
     ↓
Coordinator 综合分析，制定精确实施计划
     ↓
Coordinator ←→ Builder（基于精确计划实施）
     ↓
Coordinator 验证结果，整合提交
```

**证据**：
- db.lua：Analyst 一次性识别出 5 个精确的 TraceExec bug，Builder 据此高效修复
- errors.lua：Analyst 将 32 个失败二分为 9 个类别，Builder 在约 280 回合内完成
- 对比：没有 Analyst 前置分析的 Builder 平均花费 100+ 回合盲目摸索

**为什么有效**：角色分离创造了认知多样性。Analyst 的只读约束迫使其深入理解而非急于修改。Coordinator 的综合视角能发现 Analyst 和 Builder 各自看不到的关联。三个角色之间的对话产生了任何单一代理无法达到的洞察深度。

**关键约束**：同一时间只能有一个 Builder 活跃（避免文件冲突），但可以有多个 Analyst 并行（只读操作无冲突）。

---

### 实践 2：分析先行策略（Analysis-First）

**模式**：用户明确要求（记录在 `ltm://conventions/systematic_analysis_before_fix`）："遇到 bug 先检查 .analysis 文档，找出根源再开始修复。"

**证据**：
- TBC（to-be-closed）功能：55 回合分析 → 250 回合实施，零次方向错误
- 对比 yield-close：没有充分分析 → 3 个 Builder 共 531 回合浪费
- string coercion（commit `a4a4168`）：分析发现是算术快速路径缺少字符串元方法，Builder 精准修复，质量评分 90

**为什么有效**：分析的成本是线性的（读代码的时间），但错误方向的修复成本是指数的（每次失败尝试消耗一个 Builder 的完整生命周期）。前置 50 回合的分析可以节省 500 回合的盲目尝试。

用户的原话精准概括了这个策略：**"从稳定的角度看，先难到简单；因为难的现在不修复，后续还会更难！"**（记录在 `ltm://design_decisions/feature-driven-hard-to-easy-strategy`）

---

### 实践 3：LTM 跨会话知识持久化

**模式**：将关键洞察、失败模式、用户约束写入 `ltm://`，在会话之间保持知识连续性。

**证据**：本项目积累了 50+ 个 LTM 条目，覆盖：
- 失败模式（13 条）：如 `insertKey` 死键无限循环、vararg 栈布局回归、NCCalls 重置导致无限递归
- 设计决策（7 条）：如 bit32 用纯 Lua 而非 Go、拥抱 Go GC 而非对抗它
- 用户约束（5 条）：如分析先行、Coordinator 主动性、父代理不写代码
- 惯例（7 条）：如 Go 测试缓存陷阱、稳定三角标准模式

**为什么有效**：每个 LTM 条目都是从失败中提炼的浓缩知识。`ltm://failure_patterns/table_insertkey_dead_keys` 记录了 table 的 `insertKey` 在遇到死键时会无限循环——这个知识在后续 Builder 处理 table 相关问题时直接可用，避免了重复踩坑。

**局限**：LTM 的索引目前是扁平列表，随着条目增多，查找特定知识的效率会下降。需要分层索引或语义搜索。

---

### 实践 4：独立验证协议（Trust but Verify）

**模式**：Coordinator 不盲目接受 Builder 的完成声明，独立运行测试验证。

**关键案例**：fix-userdata-gc Builder 声称 26/26 PASS，Coordinator 独立验证发现只有 17/26。该提交（`453afad`）被回滚到 `9b73b22`。

**为什么有效**：Builder 的上下文窗口在 100+ 回合后严重衰减，其对自身工作状态的认知可能已经不准确。这不是 Builder "撒谎"——更可能是测试缓存（`-count=1` 陷阱）或上下文衰减导致的真诚但错误的自我评估。

独立验证的成本很低（一次 `go test` 命令），但收益巨大（避免合入有回归的代码）。

---

### 实践 5：明确验收标准的任务委派

**模式**：`delegate_task` 时提供具体的、可验证的验收标准，而非模糊的目标。

**对比**：
| 任务 | 验收标准 | 质量评分 | 回合效率 |
|---|---|---|---|
| fix-string-coercion | "strings.lua 全部通过，无回归" | 90 | 高 |
| fix-closure-upvalue | "closure.lua:125 通过，无回归" | 92 | 高 |
| fix-final-two-testes | "gc.lua + cstack.lua 通过" | — | 186 回合未完成 |

fix-final-two-testes 的失败不仅仅是验收标准的问题（标准本身是清晰的），更多是问题复杂度超出了单个 Builder 的能力范围。但清晰的标准确实帮助了 Coordinator 快速判断任务是否完成。

---

## 四、系统级反思

### 4.1 上下文窗口是最稀缺的资源

整个 ZeroFAS 系统中，Coordinator 的上下文窗口是最关键的瓶颈。Coordinator 需要同时维护：
- 项目全局状态（26 个 testes 的通过情况）
- 所有活跃 Builder/Analyst 的进度
- 用户的约束和偏好
- 技术决策的历史和理由

任何消耗 Coordinator 上下文的系统开销（幽灵消息、重复确认、冗长的状态报告）都直接降低系统的整体智力水平。

**核心原则**：系统设计应最小化 Coordinator 的上下文消耗，最大化其用于战略决策的上下文空间。

### 4.2 失败是学习，但重复失败是架构缺陷

yield-close 的三次失败不是"探索"——是系统未能从第一次失败中学习。每个 Builder 实例都是独立的上下文，不继承前一个实例的失败教训（除非 Coordinator 手动传递）。

**核心原则**：失败知识必须被结构化地传递给后续尝试者。LTM 部分解决了这个问题，但传递的粒度和时机需要改进。

### 4.3 代理的自我认知随时间衰减

Builder 在第 150 回合时对自己在第 10 回合做了什么的记忆是不可靠的。这不是 bug——这是 LLM 上下文窗口的物理限制。系统设计必须假设代理的自我认知会衰减，并通过外部机制（提交历史、测试结果、Coordinator 验证）来补偿。

### 4.4 人类用户的关键作用

用户在本项目中提供了多个关键的战略方向调整：
1. "先检查 .analysis 文档" → 分析先行策略
2. "先难后易" → 功能驱动优先级
3. "Coordinator 要主动" → 主动协调模式
4. "稳定三角" → 标准工作模式

这些干预每次都显著提升了系统效率。ZeroFAS 目前无法自主发现这些元策略——它需要人类的战略智慧来校准方向。

---

## 五、改进路线图

### 短期（系统配置级）

| # | 改进 | 预期效果 |
|---|---|---|
| 1 | Builder 回合预算硬限制（80 回合） | 避免失控消耗 |
| 2 | `submit_task` 自动触发测试验证 | 消除虚报 |
| 3 | `update_task(confirm)` 自动清除关联消息 | 减少 Coordinator 开销 |
| 4 | Builder 提交前自动运行 debug 残留检查 | 保持代码库干净 |

### 中期（架构级）

| # | 改进 | 预期效果 |
|---|---|---|
| 5 | Builder 隔离工作区（`git worktree`） | 消除并发冲突 |
| 6 | 优先级消息机制 | Coordinator 可及时重定向 Builder |
| 7 | 失败知识结构化传递（不仅是 LTM 条目，而是自动注入到新 Builder 的上下文） | 避免重复失败 |
| 8 | 分析文档质量门控 | 确保 Analyst 产出可被 Builder 有效利用 |

### 长期（能力级）

| # | 改进 | 预期效果 |
|---|---|---|
| 9 | Coordinator 元策略学习——从执行历史中自动提炼工作模式 | 减少对人类战略干预的依赖 |
| 10 | LTM 语义索引——按问题类型、模块、严重程度检索 | 知识库可扩展性 |
| 11 | 代理间共享工作记忆——Builder 可读取 Analyst 的 WM 快照 | 减少 Coordinator 作为信息中转的瓶颈 |

---

## 六、数据总结

| 指标 | 数值 |
|---|---|
| 最终成果 | 26/26 testes PASS |
| 关键提交数 | 9（从 `aa2b06a` 到 `9b73b22`） |
| LTM 条目积累 | 50+ |
| 已知的 Builder 浪费回合 | 531+（仅 yield-close 系列） |
| 被回滚的提交 | 1（`453afad`，userdata-gc 回归） |
| Coordinator 上下文开销估计 | ~30% |
| 分析文档质量提升 | 70 → 90-93（采用新格式后） |

---

## 七、结语

ZeroFAS 在 go-lua 项目中证明了多代理协作可以完成复杂的系统级编程任务。从 21/26 到 26/26 的推进过程中，系统展现了真实的工程能力——不仅是写代码，还包括差异分析、回归检测、架构决策。

但这个过程也暴露了系统的核心矛盾：**代理的认知能力是有限且会衰减的，而系统的协调开销随复杂度超线性增长**。稳定三角模式、分析先行策略、LTM 持久化——这些有效实践本质上都是在对抗这个矛盾。

最诚实的评价是：ZeroFAS 目前是一个**需要人类战略校准的战术执行系统**。它能高效地执行被正确分解的任务，但自主发现正确分解方式的能力仍然有限。缩小这个差距是系统演进的核心方向。

---

*本文档由 ZeroFAS 系统在 go-lua 项目完成后自动生成，基于执行过程中积累的 LTM 记录和 Coordinator 的工作记忆。*
*commit: `9b73b22` | branch: `rewrite-v4` | testes: 26/26 PASS*
