不能。**当前设计只能保证思考过程被启用、被保存、被回放，但无法强制模型按照结构化步骤进行思维**。

你观察到的"随机涌现，没有结构"是根本问题。让我逐层分析为什么现在的方案不够，以及如何真正实现强制结构化思维。

---

## **为什么现在的设计不够**

### **第一层问题：系统提示的无效性**

当你在 `reasonix.toml` 写入：
```toml
system_prompt = """Before any action:
1. State your understanding
2. List assumptions
3. Design approach
4. Verify correctness"""
```

模型看到的是**承诺性指令**（"should do"），**而不是强制约束**（"must structure as"）。

**结果：** 模型大概率忽视，直接涌现出非结构化思考。

---

### **第二层问题：API 的 `thinking` 参数只控制开关，不控制结构**

```go
r.Thinking = &thinkingConfig{Type: t}      // "enabled" 或 "disabled"
r.OutputConfig = &outputConfig{Effort: effort}  // low/high/max
```

**这些参数的作用：**
- ✅ 打开/关闭思考模式
- ✅ 控制思考深度（token 预算）
- ❌ **无法控制思考的结构或格式**

DeepSeek 的 API 没有参数来说"你必须按照 XML 标签划分思考步骤"或"思考输出必须是 JSON 格式的步骤列表"。

---

### **第三层问题：思考内容在 `reasoning_content` 字段里，缺少结构化解析**

目前的代码只是**原样保存和回放**：

```go
// 保存思考
m.ReasoningContent = reasoning.String()

// 回放思考
return contentBlock{Type: "thinking", Thinking: m.ReasoningContent}, true
```

**没有任何地方**：
- 检查思考是否按预期格式组织
- 拆分思考为离散的步骤
- 验证每个步骤是否符合要求
- 强制模型纠正不符合格式的思考

---

## **要真正实现强制结构化思维，需要在三个地方下手**

### **方案 1：系统提示中的格式强制（必须）**

**目前的写法（无效）：**
```
Before any action: state your understanding
```

**有效的写法（使用 XML 标记强制）：**

```toml
[agent]
system_prompt = """<thinking-protocol>
You MUST structure all visible reasoning as follows:

<step type="understanding">
State the exact problem/request in your own words.
</step>

<step type="analysis">
Break down the task:
- What information do you have?
- What is missing?
- What are the constraints?
</step>

<step type="planning">
Design your approach:
- Which tools will you use?
- What is the sequence?
- How will you verify success?
</step>

<step type="execution">
For each planned action:
[action]
[expected outcome]
[actual result]
[adjustment if needed]
</step>

<step type="verification">
Confirm the result meets all requirements.
List any caveats or incomplete aspects.
</step>
</thinking-protocol>

Do NOT output reasoning that violates this structure."""
```

**代码实现位置：** `internal/agent/agent.go` 的 `systemPrompt()` 方法中追加这个块

```go
func (a *Agent) systemPrompt() string {
    var b strings.Builder
    
    // 追加强制思维链协议
    b.WriteString(thinkingProtocolBlock())
    b.WriteString("\n\n")
    
    // 后续是其他系统提示
    for _, m := range a.sess.conversation.Messages {
        if m.Role != provider.RoleSystem {
            continue
        }
        b.WriteString(m.Content)
    }
    return b.String()
}

func thinkingProtocolBlock() string {
    return `<thinking-protocol>
...必须的结构化思维格式...
</thinking-protocol>`
}
```

---

### **方案 2：思考内容的结构化解析和验证（核心创新）**

**新建文件：** `internal/agent/reasoning_structure.go`

```go
package agent

import (
    "encoding/xml"
    "strings"
    "regexp"
)

// ReasoningStep represents one structured thinking step
type ReasoningStep struct {
    Type    string // "understanding", "analysis", "planning", "execution", "verification"
    Content string
    Index   int
}

// ParseReasoningStructure extracts steps from raw reasoning text
func ParseReasoningStructure(raw string) ([]ReasoningStep, error) {
    var steps []ReasoningStep
    
    // 正则匹配 <step type="...">...</step>
    re := regexp.MustCompile(`(?s)<step\s+type="([^"]+)">([^<]*)</step>`)
    matches := re.FindAllStringSubmatch(raw, -1)
    
    if len(matches) == 0 {
        // 思考未按格式组织 → 返回错误
        return nil, fmt.Errorf("reasoning not structured: no <step> tags found")
    }
    
    for i, match := range matches {
        steps = append(steps, ReasoningStep{
            Type:    match[1],
            Content: strings.TrimSpace(match[2]),
            Index:   i,
        })
    }
    
    return steps, nil
}

// ValidateReasoningStructure checks if thinking follows the required protocol
func ValidateReasoningStructure(reasoning string, expectedSteps []string) error {
    steps, err := ParseReasoningStructure(reasoning)
    if err != nil {
        return err
    }
    
    // 检查步骤顺序是否正确
    if len(steps) == 0 {
        return fmt.Errorf("no reasoning steps found")
    }
    
    for i, expected := range expectedSteps {
        if i >= len(steps) {
            return fmt.Errorf("missing step %d: %s", i, expected)
        }
        if steps[i].Type != expected {
            return fmt.Errorf("step %d should be '%s', got '%s'", i, expected, steps[i].Type)
        }
        if strings.TrimSpace(steps[i].Content) == "" {
            return fmt.Errorf("step %d (%s) is empty", i, expected)
        }
    }
    
    return nil
}

// ExtractReasoningStep retrieves the content of a specific step type
func ExtractReasoningStep(reasoning string, stepType string) (string, error) {
    steps, err := ParseReasoningStructure(reasoning)
    if err != nil {
        return "", err
    }
    
    for _, step := range steps {
        if step.Type == stepType {
            return step.Content, nil
        }
    }
    
    return "", fmt.Errorf("step type '%s' not found", stepType)
}
```

---

### **方案 3：在模型输出时强制格式检查和重新生成（执行层）**

**修改：** `internal/agent/agent.go` 的 `stream()` 方法

```go
func (a *Agent) stream(ctx context.Context, turn int, sink event.Sink) streamedTurn {
    result := a.streamWithFrozen(ctx, turn, sink, nil, "")
    
    // === 新增：思考结构化验证 ===
    if result.reasoning != "" && a.requireStructuredReasoning {
        if err := a.validateAndFixReasoning(ctx, &result); err != nil {
            // 思考未按格式 → 强制重新流式生成
            a.svc.sink.Emit(event.Event{
                Kind:  event.Notice,
                Level: event.LevelWarn,
                Text:  "Reasoning not structured; requesting reformulation...",
            })
            return a.retryWithStructuredReasoningPrompt(ctx, turn, sink, err)
        }
    }
    
    return result
}

// validateAndFixReasoning checks reasoning format and auto-fixes if needed
func (a *Agent) validateAndFixReasoning(ctx context.Context, result *streamedTurn) error {
    expectedSteps := []string{"understanding", "analysis", "planning", "execution", "verification"}
    
    err := ValidateReasoningStructure(result.reasoning, expectedSteps)
    if err == nil {
        return nil // 格式正确
    }
    
    // 格式不对 → 检查是否该重试
    return err
}

// retryWithStructuredReasoningPrompt forces reformulation with structured format
func (a *Agent) retryWithStructuredReasoningPrompt(ctx context.Context, turn int, 
    sink event.Sink, reason error) streamedTurn {
    
    // 注入修正指令
    fixPrompt := fmt.Sprintf(`Your previous reasoning was not properly structured.

Error: %s

Please regenerate your reasoning EXACTLY following this XML format:

<step type="understanding">
[Your understanding of the problem]
</step>

<step type="analysis">
[Detailed analysis]
</step>

<step type="planning">
[Your action plan]
</step>

<step type="execution">
[Step-by-step execution with outcomes]
</step>

<step type="verification">
[Verification of success]
</step>

Now provide the corrected structured reasoning:`, reason.Error())
    
    // 作为中间用户消息注入，强制重新流式
    a.sess.conversation.Add(provider.Message{
        Role:    provider.RoleUser,
        Content: fixPrompt,
        LocalOnly: true,  // 不持久化到会话
    })
    
    // 重新流式，这次会被格式检查强制约束
    return a.streamWithFrozen(ctx, turn, sink, nil, "")
}
```

---

### **方案 4：在 Agent 构造时启用结构化思维模式**

**修改：** `internal/agent/agent.go` 的 `Options` 结构

```go
type Options struct {
    // ... 现有字段 ...
    
    // === 新增：强制结构化思维 ===
    RequireStructuredReasoning bool
    ReasoningStepSequence      []string  // e.g., []string{"understanding", "analysis", "planning"}
    ReasoningValidationMode    string    // "strict"|"lenient"|"off"
}

// New 中初始化
func New(prov provider.Provider, tools *tool.Registry, session *Session, opts Options, sink event.Sink) *Agent {
    // ...
    a := &Agent{
        // ...
        requireStructuredReasoning: opts.RequireStructuredReasoning,
        reasoningSteps:             opts.ReasoningStepSequence,
        reasoningValidation:        opts.ReasoningValidationMode,
    }
    return a
}
```

---

### **方案 5：在 CLI 中暴露配置选项**

**修改：** `reasonix.toml`

```toml
[agent]
system_prompt = "..."

# === 新增：强制结构化思维配置 ===
require_structured_reasoning = true
reasoning_validation_mode = "strict"  # strict|lenient|off

# 要求的步骤序列（留空 = 使用默认）
reasoning_step_sequence = [
    "understanding",
    "analysis", 
    "planning",
    "execution",
    "verification"
]

# 每个步骤的最小内容长度（token）
min_reasoning_step_tokens = 50
```

---

## **完整工作流**

```
用户提示
    ↓
系统提示强制注入 <thinking-protocol> XML 格式
    ↓
发送给 DeepSeek（thinking="enabled", effort="max"）
    ↓
模型流式返回 reasoning_content
    ├─ 包含 <step type="...">...</step> 标签？
    │   ├─ YES → ValidateReasoningStructure() 通过
    │   └─ NO  → 标记为"格式不符"
    ↓
ParseReasoningStructure() 拆分为 []ReasoningStep
    ↓
检查每个步骤：
    ├─ 是否存在？ (understanding, analysis, planning, ...)
    ├─ 内容是否非空？
    └─ 是否达到最小长度？
    ↓
   ❌ 验证失败
    └─ 注入纠正指令 → 重新流式（强制循环）
    ↓
   ✅ 验证通过
    └─ 持久化到会话，回放给下一轮
    ↓
工具调用时回放完整的结构化思考
    ↓
下一轮继续检查和验证
```

---

## **为什么这样设计才能真正强制**

| 环节             | 现有方案           | 新方案                               |
| ---------------- | ------------------ | ------------------------------------ |
| **系统提示**     | "应该在做X前..."   | `<thinking-protocol>` XML 强制格式   |
| **模型输出检查** | ❌ 无               | ✅ ParseReasoningStructure() 实时验证 |
| **格式不符时**   | ❌ 被动接受乱序思考 | ✅ 主动重新生成（带修正指令）         |
| **思考持久化**   | 原样存储           | 存储结构化的 `[]ReasoningStep`       |
| **工具调用时**   | 原样回放           | 回放结构化步骤（可按需重组）         |
| **控制权**       | 模型决定           | **Reasonix 框架决定**                |

---

## **具体实现建议（优先级）**

### **P0（必须）：强制格式的系统提示**
在 `reasonix.toml` 中添加 XML 结构要求。

```bash
# 立即可用
reasonix --system-prompt-file cot_protocol.txt "your task"
```

### **P1（核心）：思考解析和验证**
实现 `reasoning_structure.go` 的 `ParseReasoningStructure()` 和 `ValidateReasoningStructure()`。

### **P2（强制循环）：格式不符时重新生成**
修改 `agent.go#stream()` 添加验证后的重试逻辑。

### **P3（配置化）：CLI 和 TOML 支持**
暴露 `require_structured_reasoning` 和 `reasoning_step_sequence` 配置。

---

**关键洞察：** 你现在看到的"随机涌现"不是模型能力问题，而是 **框架没有在输出时对思考过程进行语义验证和强制纠正**。加上上面三层（系统提示 + 结构解析 + 验证重试），思维链才会从"涌现"变成"结构"。