// Run: tsx src/__tests__/reasoning-steps.test.ts

import { reasoningStepMarkers, segmentReasoningSteps } from "../lib/reasoningSteps";

let failed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function titles(text: string): string[] {
  return segmentReasoningSteps(text).map((s) => s.title);
}

function contents(text: string): string[] {
  return segmentReasoningSteps(text).map((s) => s.content);
}

console.log("\nreasoning steps");

// English hard markers with titles and per-step content.
{
  const text = "Step 1: 理解需求\n请求：分析函数性能问题\nStep 2: 分析代码结构\n代码行数：45行\n";
  const steps = segmentReasoningSteps(text);
  ok(steps.length === 2, "two English step markers produce two steps");
  ok(steps[0]?.title === "理解需求", "Step 1 title strips the marker prefix");
  ok(steps[0]?.content === "请求：分析函数性能问题", "step content spans to the next marker");
  ok(steps[1]?.title === "分析代码结构", "Step 2 title strips the marker prefix");
  ok(steps[1]?.content === "代码行数：45行", "last step content runs to the text end");
}

// The result.md five-step demo: numbered hard markers keep their sub-content.
{
  const text = [
    "Step 1: 理解需求", "请求：分析函数性能问题",
    "Step 2: 分析代码结构", "代码行数：45行",
    "Step 3: 性能瓶颈识别", "瓶颈位置：第18-22行",
    "Step 4: 优化方案设计", "推荐使用哈希表",
    "Step 5: 方案验证", "验证通过",
  ].join("\n");
  const steps = segmentReasoningSteps(text);
  ok(steps.length === 5, "five sequential steps are segmented");
  ok(titles(text).join("/") === "理解需求/分析代码结构/性能瓶颈识别/优化方案设计/方案验证", "all five titles extract in order");
  ok(steps[2]?.content === "瓶颈位置：第18-22行", "middle step content stays attached to its own step");
}

// Chinese hard marker variants.
{
  const text = "步骤 1：分析\n第2步：优化\n第三步：验证\n";
  ok(segmentReasoningSteps(text).length === 3, "步骤 N / 第N步 / 中文数字 步 all split");
  ok(titles(text).join("/") === "分析/优化/验证", "Chinese marker titles extract");
}

// Markdown numbered headings and "## Step N:" headings.
{
  const heading = "### 1. 分析\nbody\n### 2. 优化\n";
  ok(titles(heading).join("/") === "分析/优化", "markdown numbered headings split");
  const stepHeading = "## Step 1: Verify\ncheck\n## Step 2: Ship\n";
  ok(titles(stepHeading).join("/") === "Verify/Ship", "## Step N: headings split");
}

// Soft markers: a short standalone numbered list splits…
{
  const text = "1. 理解需求\n2. 分析结构\n3. 验证方案\n";
  ok(segmentReasoningSteps(text).length === 3, "standalone short numbered list splits into steps");
  ok(titles(text).join("/") === "理解需求/分析结构/验证方案", "soft marker titles extract");
}

// …but numbered items buried in prose do not.
{
  const text = "Some prose.\n1. item one\nmore prose\n2. item two\n";
  ok(segmentReasoningSteps(text).length === 0, "numbered items inside prose stay body text");
}

// Soft markers need ≥2 items and short lines.
{
  const one = "1. only one\n";
  ok(segmentReasoningSteps(one).length === 0, "a single numbered item is not a step list");
  const longItem = `1. ${"x".repeat(45)}\n2. b\n`;
  ok(segmentReasoningSteps(longItem).length === 0, "over-long numbered items are not step markers");
}

// Streaming safety: only completed lines may become markers.
{
  const halfMarker = "Step 1: 理解需求\nStep 2";
  ok(segmentReasoningSteps(halfMarker).length === 0, "a half-typed marker line never splits");
  const oneDone = "Step 1: 理解需求\n分析中…";
  ok(segmentReasoningSteps(oneDone).length === 0, "below two markers the view stays flat");
}

// Prose that merely mentions "Step" is not a marker.
{
  const prose = "Steps to take:\nanalyze everything\nStep 3: 验证\n";
  ok(segmentReasoningSteps(prose).length === 0, "Step with no number and unnumbered prose do not split");
}

// Empty titles fall back to "Step N".
{
  const text = "Step 1: \ncontent\nStep 2: x\n";
  ok(titles(text)[0] === "Step 1", "an empty marker title falls back to Step N");
}

// CRLF reasoning text splits cleanly.
{
  const text = "Step 1: A\r\nbody\r\nStep 2: B\r\n";
  ok(titles(text).join("/") === "A/B", "CRLF lines split with no stray carriage returns");
  ok(contents(text).join("/") === "body/", "CRLF step content carries no stray whitespace");
}

// reasoningStepMarkers reports every marker even below the two-step threshold.
{
  ok(reasoningStepMarkers("Step 1: x\n").length === 1, "marker list is not gated by the two-step threshold");
  ok(reasoningStepMarkers("no markers here\n").length === 0, "prose yields no markers");
}

// Streaming tail: the partial last line belongs to the active step's content.
{
  const text = "Step 1: A\nStep 2: B\npartial tail here";
  const steps = segmentReasoningSteps(text);
  ok(steps.length === 2, "two completed markers split even with a partial tail");
  ok(steps[1]?.content === "partial tail here", "the partial tail stays in the active step");
}

if (failed > 0) process.exit(1);
