## **改造后的思考过程步骤展示演示**

### **完整对话框视图**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Reasonix - DeepSeek-Reasonix                                        [- ▢ ✕] │
├─────────────────────────────────────────────────────────────────────────────┤
│ 📁 project  🔍 sessions  💬 chat         [⚙️ settings]  [🔔 notifications]   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  [Your Workspace] / [Topic] / Session 1                                      │
│                                                                               │
│  ─────────────────────────────────────────────────────────────────────────   │
│                                                                               │
│  👤 You                                                          [14:32 PM]   │
│                                                                               │
│  帮我分析这个函数的性能问题，然后给出优化方案                                │
│                                                                               │
│  ─────────────────────────────────────────────────────────────────────────   │
│                                                                               │
│  🤖 Assistant                                     [thinking: 23s] [cost: TBD]│
│                                                                               │
│  ▼ Thinking (4/5 steps completed)  ⏱️ 23 seconds                             │
│  ├─ ✓ Step 1: 理解需求                                       [完成]         │
│  ├─ ✓ Step 2: 分析代码结构                                   [完成]         │
│  ├─ ✓ Step 3: 性能瓶颈识别                                   [完成]         │
│  ├─ ● Step 4: 优化方案设计                    ● Thinking...   [进行中]      │
│  │                                                                            │
│  │  <Expanded - Streaming Content>                                           │
│  │                                                                            │
│  │  当前在分析时间复杂度...                                                   │
│  │  - 原始算法: O(n²)                                                         │
│  │  - 优化后: O(n log n)                                                      │
│  │  - 推荐使用哈希表来缓存...                                                 │
│  │                                                                            │
│  └─ ○ Step 5: 方案验证                                        [未开始]       │
│                                                                               │
│  ─────────────────────────────────────────────────────────────────────────   │
│                                                                               │
│  📝 Input Composer                                                           │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ 你的下一步指令... [@files] [/skill] [/goal]                             │ │
│  │                                                                          │ │
│  │                                                                          │ │
│  │                                                  [➤ Send] [⌘K Options] │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
│  ─────────────────────────────────────────────────────────────────────────   │
│                                                                               │
│  Status: ⚙️ thinking (4 of 5 steps)  •  💾 8,234 tokens  •  💰 ¥0.12 (est)  │
│                                                                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## **详细交互流程**

### **第一阶段：思考开始（0-5秒）**

```
🤖 Assistant                                    [thinking: 2s] [cost: calculating]

▼ Thinking (0/5 steps detected)  ⏱️ 2 seconds

📊 Step detection in progress...

  初始化思考过程...
```

### **第二阶段：步骤逐步出现（5-25秒）**

```
🤖 Assistant                                   [thinking: 12s] [cost: calculating]

▼ Thinking (3/5 steps completed)  ⏱️ 12 seconds

├─ ✓ Step 1: 理解需求                                        [完成 · 2.3s]
│  ├─ 请求：分析函数性能问题
│  ├─ 输入：Python函数代码
│  └─ 目标：找到瓶颈并优化
│
├─ ✓ Step 2: 分析代码结构                                   [完成 · 3.1s]
│  ├─ 代码行数：45行
│  ├─ 时间复杂度：O(n²)
│  ├─ 空间复杂度：O(n)
│  └─ 使用了嵌套循环遍历列表
│
├─ ✓ Step 3: 性能瓶颈识别                                   [完成 · 4.2s]
│  ├─ 瓶颈位置：第18-22行
│  ├─ 原因：重复查询列表
│  ├─ 影响：对于大型数据集性能线性下降
│  └─ 优先级：高
│
└─ ● Step 4: 优化方案设计                    ● Thinking...   [进行中 · 2.1s]
   └─ 正在推导最优算法...
```

### **第三阶段：完成阶段（思考结束）**

```
🤖 Assistant                                   [thinking: 24s] [cost: ¥0.18]

▼ Thinking (5/5 steps completed)  ⏱️ 24 seconds

├─ ✓ Step 1: 理解需求                                        [完成 · 2.3s]
├─ ✓ Step 2: 分析代码结构                                   [完成 · 3.1s]
├─ ✓ Step 3: 性能瓶颈识别                                   [完成 · 4.2s]
├─ ✓ Step 4: 优化方案设计                                   [完成 · 8.5s]
└─ ✓ Step 5: 方案验证                                        [完成 · 5.9s]

✅ 分析完成 · 总耗时 24 秒


## 优化方案总结

### 问题分析
原始代码在第18-22行使用了嵌套循环，对每个元素都遍历整个列表进行查询，
导致时间复杂度为 O(n²)。对于百万级数据，这会导致性能严重下降。

### 优化方案

**方案一：使用哈希表（推荐）**
- 时间复杂度：O(n)
- 空间复杂度：O(n)
- 提升：5000 倍（100万条数据）
- 代码改动：最小化

**方案二：排序 + 二分查找**
- 时间复杂度：O(n log n)
- 空间复杂度：O(1)
- 提升：1000 倍
- 适用场景：空间有限

### 具体代码
```python
# 优化前
def find_duplicates(items):
    result = []
    for item in items:
        if item in items:  # ❌ O(n) 查询，嵌套导致 O(n²)
            result.append(item)
    return result

# 优化后
def find_duplicates(items):
    seen = set(items)  # ✅ O(n) 一次扫描
    return list(seen)  # O(1) 查询
```

─────────────────────────────────────────────────────────────────────────
```

---

## **用户交互：点击某一步查看细节**

用户点击 "Step 3: 性能瓶颈识别"

```
┌─ Step 3: 性能瓶颈识别                                   [完成 · 4.2s] ─┐
│                                                                          │
│ 分析范围：函数行18-22行                                                 │
│                                                                          │
│ 瓶颈类型：嵌套循环查询                                                   │
│                                                                          │
│ 代码片段：                                                               │
│ ────────────────────────────────────────────────────────────────────   │
│ for i in range(len(items)):                    # ← 外层循环 O(n)      │
│     for j in range(len(items)):                # ← 内层循环 O(n)      │
│         if items[i] == items[j] and i != j:   # ← 查询 O(1) × O(n²)  │
│             duplicates.append(items[i])                                │
│ ────────────────────────────────────────────────────────────────────   │
│                                                                          │
│ 性能影响分析：                                                           │
│ • 输入大小 100 条：100² = 10,000 次比较                                 │
│ • 输入大小 1000 条：1000² = 1,000,000 次比较                            │
│ • 输入大小 100万条：跳过... 🔴 不可接受                                 │
│                                                                          │
│ 根本原因：算法设计不合理，暴力穷举所有组合                               │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## **用户与进行中的步骤交互**

### **折叠已完成步骤以减少噪音**

点击 "★" 按钮**折叠**步骤 1-3：

```
🤖 Assistant                                   [thinking: 24s] [cost: ¥0.18]

▼ Thinking (5/5 steps completed)  ⏱️ 24 seconds

⋯ [3 completed steps collapsed]  ★ (点击展开)

└─ ✓ Step 5: 方案验证                                        [完成 · 5.9s]

✅ 分析完成 · 总耗时 24 秒
```

---

## **工具调用时的步骤展示**

当思考后进入**工具调用阶段**（假设有代码改写工具）：

```
🤖 Assistant

✅ 思考完成 [24s]

Now executing the optimization plan...

🔧 Tool: write_file
   📄 File: performance_utils.py
   ✍️ Writing optimized function...
   ⏳ Processing...

🔧 Tool: run_tests
   🧪 Running performance tests...
   ✅ Test 1: Basic functionality... PASSED
   ✅ Test 2: Edge cases... PASSED
   ⏳ Test 3: Performance benchmark... (running)

   Before: 2.34s for 100k items
   After: 0.004s for 100k items
   ✨ Speedup: 585x
```

---

## **核心 UI 组件定义**

### **结构化思考卡片（StructuredReasoningStep.tsx）**

```typescript
interface ReasoningStepProps {
  index: number;           // 1, 2, 3...
  title: string;           // "理解需求", "分析代码"...
  status: 'pending' | 'streaming' | 'complete';
  startTime?: number;
  completedAt?: number;
  content?: string;        // 当展开时显示的详细内容
  isExpanded?: boolean;
}

const ReasoningStep: React.FC<ReasoningStepProps> = ({
  index,
  title,
  status,
  startTime,
  completedAt,
  content,
  isExpanded = true,
}) => {
  const statusIcon = {
    pending: "○",        // 未开始
    streaming: "●",      // 进行中（带呼吸动画）
    complete: "✓",       // 完成
  }[status];
  
  const duration = startTime && completedAt 
    ? ((completedAt - startTime) / 1000).toFixed(1) + "s"
    : status === 'streaming' ? "..." : "";
  
  return (
    <div className={`reasoning-step step-${status}`}>
      <div className="step-header" onClick={toggleExpand}>
        <span className="step-icon">{statusIcon}</span>
        <span className="step-number">Step {index}</span>
        <span className="step-title">{title}</span>
        <span className="step-duration">[{duration}]</span>
        <span className="step-chevron">▼</span>
      </div>
      
      {isExpanded && (
        <div className="step-content">
          <div className="step-text">{content}</div>
        </div>
      )}
    </div>
  );
};
```

---

## **样式（StructuredReasoningStep.css）**

```css
.reasoning-step {
  border-left: 3px solid #e0e0e0;
  margin: 8px 0;
  transition: all 0.2s ease;
}

.reasoning-step.step-complete {
  border-left-color: #4caf50;
  background: rgba(76, 175, 80, 0.04);
}

.reasoning-step.step-streaming {
  border-left-color: #5e7ce0;
  background: rgba(94, 124, 224, 0.08);
  box-shadow: 0 0 12px rgba(94, 124, 224, 0.2);
}

.step-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  user-select: none;
}

.step-header:hover {
  background: rgba(0, 0, 0, 0.04);
}

.step-icon {
  width: 16px;
  text-align: center;
}

.reasoning-step.step-streaming .step-icon {
  animation: pulse 1s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.step-number {
  color: #666;
  font-size: 12px;
  font-weight: 400;
}

.step-title {
  flex: 1;
  color: #333;
}

.step-duration {
  color: #999;
  font-size: 12px;
}

.step-chevron {
  width: 12px;
  text-align: center;
  color: #999;
  transition: transform 0.2s;
}

.step-chevron.open {
  transform: rotate(180deg);
}

.step-content {
  padding: 8px 12px 12px 32px;
  background: rgba(0, 0, 0, 0.02);
  border-top: 1px solid #f0f0f0;
  font-size: 12px;
  line-height: 1.6;
  color: #555;
  white-space: pre-wrap;
  word-wrap: break-word;
}
```

---

## **关键数据流（事件/消息）**

```typescript
// 从后端发送的事件序列
type ReasoningStepEvent = {
  kind: "reasoning_step";
  stepIndex: number;
  title: string;
  status: "start" | "update" | "complete";
  content?: string;
  duration?: number;  // ms
};

// 序列示例
[
  { kind: "reasoning_step", stepIndex: 1, status: "start", title: "理解需求" },
  { kind: "reasoning", text: "分析用户请求..." },
  { kind: "reasoning", text: "输入是一个Python函数..." },
  { kind: "reasoning_step", stepIndex: 1, status: "complete", duration: 2300 },
  
  { kind: "reasoning_step", stepIndex: 2, status: "start", title: "分析代码结构" },
  { kind: "reasoning", text: "检查函数行数..." },
  { kind: "reasoning", text: "识别出嵌套循环..." },
  { kind: "reasoning_step", stepIndex: 2, status: "complete", duration: 3100 },
  
  // ... 更多步骤
]
```

---

## **完整的桌面端截图模拟（文字版）**

```
╔═════════════════════════════════════════════════════════════════════════════╗
║                                                                             ║
║  You:  分析这个函数性能问题                      [14:32]  [Turn 1]       ║
║                                                                             ║
║  ─────────────────────────────────────────────────────────────────────────  ║
║                                                                             ║
║  Assistant:  [Running...] 思考中  |  Effort: max  |  Token: 8,234          ║
║                                                                             ║
║  🧠 Thinking Progress                                    ⏱️  24 seconds    ║
║  ┌─────────────────────────────────────────────────────────────────────────┐║
║  │ ▼ Thinking (5/5 steps completed)                                        ││
║  │                                                                          ││
║  │ ├─ ✓ Step 1: 理解需求              [2.3s]     [点击展开详情]          ││
║  │ │  └─ 请求分析一个Python函数...                                        ││
║  │ │                                                                       ││
║  │ ├─ ✓ Step 2: 分析代码结构          [3.1s]     [点击展开详情]          ││
║  │ │  └─ 代码行数：45 · 嵌套循环：是 · 复杂度：O(n²)                      ││
║  │ │                                                                       ││
║  │ ├─ ✓ Step 3: 性能瓶颈识别          [4.2s]     [展开中 ▼]              ││
║  │ │                                                                       ││
║  │ │  ┌─ 瓶颈详情 ──────────────────────────────────────────────────────┐││
║  │ │  │ 位置：18-22 行                                                   │││
║  │ │  │ 原因：重复遍历列表                                               │││
║  │ │  │ 影响：n² 复杂度                                                  │││
║  │ │  │                                                                  │││
║  │ │  │ 代码片段：                                                       │││
║  │ │  │   for i in range(len(items)):     # O(n)                        │││
║  │ │  │       for j in range(len(items)): # O(n) → O(n²)               │││
║  │ │  │           if items[i] == items[j]:                              │││
║  │ │  │               ...                                               │││
║  │ │  └──────────────────────────────────────────────────────────────────┘││
║  │ │                                                                       ││
║  │ ├─ ✓ Step 4: 优化方案设计          [8.5s]     [点击展开详情]          ││
║  │ │  └─ 推荐哈希表方案 · 性能提升 5000x                                 ││
║  │ │                                                                       ││
║  │ └─ ✓ Step 5: 方案验证              [5.9s]     [点击展开详情]          ││
║  │    └─ 验证通过 · 功能正确 · 性能达标                                  ││
║  │                                                                          ││
║  │  📊 Summary: 24s total · 5 steps analyzed · Ready for output            ││
║  └─────────────────────────────────────────────────────────────────────────┘║
║                                                                             ║
║  ─────────────────────────────────────────────────────────────────────────  ║
║                                                                             ║
║  根据以上分析，我来为你提供优化方案。                                      ║
║                                                                             ║
║  ## 问题分析                                                                ║
║                                                                             ║
║  你的代码在第 18-22 行使用了嵌套循环遍历列表：                              ║
║  - 外层循环：O(n)                                                           ║
║  - 内层循环：O(n)                                                           ║
║  - 总复杂度：O(n²)                                                          ║
║                                                                             ║
║  这意味着对于 100 万条数据，需要进行 1 万亿次比较，性能完全不可接受。      ║
║                                                                             ║
║  ## 优化方案                                                                ║
║                                                                             ║
║  **推荐方案：使用哈希表**                                                   ║
║  - 时间复杂度：O(n)                                                         ║
║  - 性能提升：5000 倍（百万级数据）                                          ║
║  - 代码改动最小                                                             ║
║                                                                             ║
║  ```python                                                                  ║
║  # 优化前 - O(n²)                                                           ║
║  def find_duplicates(items):                                                ║
║      result = []                                                            ║
║      for item in items:                                                     ║
║          if item in items:  # ❌ 嵌套查询导致 O(n²)                         ║
║              result.append(item)                                            ║
║      return result                                                          ║
║                                                                             ║
║  # 优化后 - O(n)                                                            ║
║  def find_duplicates(items):                                                ║
║      seen = set(items)      # ✅ O(n) 一次扫描构建哈希表                    ║
║      return list(seen)      # O(1) 查询时间                                 ║
║  ```                                                                        ║
║                                                                             ║
║  ─────────────────────────────────────────────────────────────────────────  ║
║                                                                             ║
║  🔒 Turn completed  |  📊 Usage: 8,234 tokens  |  💰 Cost: ¥0.18           ║
║                                                                             ║
║  ═════════════════════════════════════════════════════════════════════════  ║
║                                                                             ║
║  💬 Composer: 你的下一步... [send]                                         ║
║                                                                             ║
╚═════════════════════════════════════════════════════════════════════════════╝
```

---

## **总结：改造后的完整体验**

| 方面             | 改造前           | 改造后              |
| ---------------- | ---------------- | ------------------- |
| **思考过程显示** | 一大段原始文本   | 5 个清晰的步骤卡片  |
| **进度感知**     | 无法知道进度     | 实时显示"3/5"完成   |
| **交互性**       | 只能看全部或不看 | 可展开/折叠每个步骤 |
| **时间追踪**     | 无               | 每步耗时显示        |
| **用户控制**     | 被动接收         | 主动探索细节        |
| **可读性**       | 低（密集文本）   | 高（结构化卡片）    |

**这就是你要的完整演示** — 可交互、真实、立即生效的桌面端思考过程步骤展示。