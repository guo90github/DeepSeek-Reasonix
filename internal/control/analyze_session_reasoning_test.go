package control

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type sessionAuditTestProvider struct {
	requests []provider.Request
	verdicts []string
}

func (p *sessionAuditTestProvider) Name() string { return "session-audit-test" }

func (p *sessionAuditTestProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.requests = append(p.requests, req)
	verdict := ""
	if len(p.verdicts) > 0 {
		verdict = p.verdicts[0]
		p.verdicts = p.verdicts[1:]
	}
	out := make(chan provider.Chunk, 2)
	go func() {
		defer close(out)
		select {
		case out <- provider.Chunk{Type: provider.ChunkText, Text: verdict}:
		case <-ctx.Done():
			return
		}
		out <- provider.Chunk{Type: provider.ChunkDone}
	}()
	return out, nil
}

func sessionAuditTestController(t *testing.T, stub *sessionAuditTestProvider) *Controller {
	t.Helper()
	return &Controller{
		selection: modelSelection{ref: "session/model"},
		audit: auditConfig{
			model: "audit/qwen",
			providerResolver: func(ref string) (provider.Provider, error) {
				return stub, nil
			},
		},
		sink: event.Discard,
	}
}

func sessionAuditTurns(count, charsPerTurn int) []SessionAuditTurn {
	turns := make([]SessionAuditTurn, 0, count)
	for i := 1; i <= count; i++ {
		turns = append(turns, SessionAuditTurn{
			Turn:      i,
			Prompt:    "第 " + itoa(i) + " 轮要求",
			Reasoning: strings.Repeat("思", charsPerTurn),
		})
	}
	return turns
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestBuildSessionAuditSegmentsGroupsInOrder(t *testing.T) {
	segments := buildSessionAuditSegments(sessionAuditTurns(20, 300), 0)
	if len(segments) != 3 {
		t.Fatalf("segments = %d, want 3", len(segments))
	}
	wantSizes := []int{7, 7, 6}
	at := 1
	for i, seg := range segments {
		if len(seg) != wantSizes[i] {
			t.Fatalf("segment %d size = %d, want %d", i+1, len(seg), wantSizes[i])
		}
		for _, turn := range seg {
			if turn.turn != at {
				t.Fatalf("segment %d holds turn %d, want %d (order must be preserved)", i+1, turn.turn, at)
			}
			if turn.truncated {
				t.Fatalf("turn %d marked truncated, want intact", turn.turn)
			}
			at++
		}
	}
	if at != 21 {
		t.Fatalf("audited turns = %d, want 20", at-1)
	}
}

func TestBuildSessionAuditSegmentsCutsLongTurns(t *testing.T) {
	segments := buildSessionAuditSegments(sessionAuditTurns(4, 20000), 0)
	if len(segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(segments))
	}
	for _, turn := range segments[0] {
		if got := len([]rune(turn.reasoning)); got != reasoningAuditMaxCharsDefault {
			t.Fatalf("turn %d length = %d, want %d", turn.turn, got, reasoningAuditMaxCharsDefault)
		}
		if !turn.truncated {
			t.Fatalf("turn %d not marked truncated after a cut", turn.turn)
		}
	}
}

func TestBuildSessionAuditSegmentsDropsTurnsWithoutReasoning(t *testing.T) {
	turns := []SessionAuditTurn{
		{Turn: 1, Reasoning: "有思考"},
		{Turn: 2, Reasoning: "   "},
		{Turn: 3, Reasoning: "也有思考"},
	}
	segments := buildSessionAuditSegments(turns, 0)
	if len(segments) != 1 || len(segments[0]) != 2 {
		t.Fatalf("segments = %+v, want one segment with 2 turns", segments)
	}
	if segments[0][0].turn != 1 || segments[0][1].turn != 3 {
		t.Fatalf("turn numbers = %d,%d, want 1,3", segments[0][0].turn, segments[0][1].turn)
	}
}

func TestSessionAuditGroupSizesStayBounded(t *testing.T) {
	sizes := sessionAuditGroupSizes(100)
	if len(sizes) != sessionAuditMaxSegments {
		t.Fatalf("groups = %d, want %d", len(sizes), sessionAuditMaxSegments)
	}
	total := 0
	for _, size := range sizes {
		if size <= 0 {
			t.Fatalf("group size %d <= 0", size)
		}
		total += size
	}
	if total != 100 {
		t.Fatalf("group sizes sum = %d, want 100", total)
	}
}

func TestAuditSessionReasoningRunsBothPasses(t *testing.T) {
	stub := &sessionAuditTestProvider{verdicts: []string{
		`{"turns":[{"turn":1,"score":0.8,"invalid_inference":1,"conclusion":"先做 A","explanation":"一次无效推理","findings":[{"type":"invalid_inference","quote":"7×8=54"}]},{"turn":2,"score":1,"conclusion":"沿用 A","explanation":"顺畅"}]}`,
		`{"score":0.72,"trend":"degrading","explanation":"轮 2 与前轮结论冲突","issues":[{"type":"cross_turn_contradiction","turns":[2,1],"note":"轮 2 推翻了轮 1","quote":"7×8=54"}]}`,
	}}
	c := sessionAuditTestController(t, stub)
	var events []SessionAuditEvent
	got, err := c.AuditSessionReasoning(context.Background(), sessionAuditTurns(2, 200), func(ev SessionAuditEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("AuditSessionReasoning: %v", err)
	}
	if !got.Audited || got.TurnCount != 2 || got.SegmentCount != 1 {
		t.Fatalf("totals = %+v, want audited 2 turns in 1 segment", got)
	}
	if got.Score != 0.72 || got.Trend != "degrading" {
		t.Fatalf("session verdict = %g/%s, want 0.72/degrading", got.Score, got.Trend)
	}
	if len(got.Issues) != 1 || got.Issues[0].Type != "cross_turn_contradiction" {
		t.Fatalf("issues = %+v, want one cross-turn contradiction", got.Issues)
	}
	if got.Issues[0].Turns[0] != 1 || got.Issues[0].Turns[1] != 2 {
		t.Fatalf("issue turns = %v, want ascending [1 2]", got.Issues[0].Turns)
	}
	if len(got.Turns) != 2 || got.Turns[0].Issues != 1 || got.Turns[0].Score != 0.8 {
		t.Fatalf("turn rows = %+v, want turn 1 with one issue scored 0.8", got.Turns)
	}
	if len(stub.requests) != 2 {
		t.Fatalf("evaluator calls = %d, want 2 (one segment + one review)", len(stub.requests))
	}
	if stub.requests[0].Messages[0].Content != auditSegmentPromptContent {
		t.Fatal("segment call did not use the batch prompt")
	}
	if !strings.Contains(stub.requests[0].Messages[1].Content, "[轮 1]") {
		t.Fatalf("segment input = %q, want tagged turns", stub.requests[0].Messages[1].Content)
	}
	if stub.requests[1].Messages[0].Content != auditSessionPromptContent {
		t.Fatal("review call did not use the cross-turn prompt")
	}
	if !strings.Contains(stub.requests[1].Messages[1].Content, "turn 1 | score 0.80") {
		t.Fatalf("review input = %q, want the per-turn rows", stub.requests[1].Messages[1].Content)
	}
	if !strings.Contains(stub.requests[1].Messages[1].Content, "7×8=54") {
		t.Fatal("review input dropped the quoted excerpts")
	}
	kinds := map[string]int{}
	for _, ev := range events {
		kinds[ev.Kind]++
	}
	if kinds["request"] != 2 || kinds["step_done"] != 2 || kinds["step_failed"] != 0 {
		t.Fatalf("event kinds = %v, want 2 requests and 2 step_done", kinds)
	}
}

func TestAuditSessionReasoningKeepsUnreportedTurns(t *testing.T) {
	stub := &sessionAuditTestProvider{verdicts: []string{
		`{"turns":[{"turn":1,"score":1,"conclusion":"ok"}]}`,
		`{"score":0.9,"trend":"stable","explanation":"基本顺畅","issues":[]}`,
	}}
	c := sessionAuditTestController(t, stub)
	got, err := c.AuditSessionReasoning(context.Background(), sessionAuditTurns(2, 100), nil)
	if err != nil {
		t.Fatalf("AuditSessionReasoning: %v", err)
	}
	if len(got.Turns) != 2 {
		t.Fatalf("turn rows = %d, want 2 (the unreported turn must not vanish)", len(got.Turns))
	}
	if got.Turns[1].Explanation != "该轮未返回评审结果" {
		t.Fatalf("turn 2 explanation = %q, want the explicit gap marker", got.Turns[1].Explanation)
	}
}

func TestAuditSessionReasoningRejectsMismatchedTurnNumbers(t *testing.T) {
	stub := &sessionAuditTestProvider{verdicts: []string{
		`{"turns":[{"turn":99,"score":1}]}`,
	}}
	c := sessionAuditTestController(t, stub)
	_, err := c.AuditSessionReasoning(context.Background(), sessionAuditTurns(2, 100), nil)
	if err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("err = %v, want a turn-number mismatch error", err)
	}
}

func TestAuditSessionReasoningRejectsEmptyReasoning(t *testing.T) {
	stub := &sessionAuditTestProvider{}
	c := sessionAuditTestController(t, stub)
	_, err := c.AuditSessionReasoning(context.Background(), []SessionAuditTurn{{Turn: 1}}, nil)
	if err == nil || !strings.Contains(err.Error(), "no auditable reasoning") {
		t.Fatalf("err = %v, want the empty-input error", err)
	}
	if len(stub.requests) != 0 {
		t.Fatalf("evaluator calls = %d, want none", len(stub.requests))
	}
}

func TestAuditSessionReasoningReportsSegmentFailure(t *testing.T) {
	stub := &sessionAuditTestProvider{verdicts: []string{"not json"}}
	c := sessionAuditTestController(t, stub)
	var failed string
	_, err := c.AuditSessionReasoning(context.Background(), sessionAuditTurns(2, 100), func(ev SessionAuditEvent) {
		if ev.Kind == "step_failed" {
			failed = ev.Error
		}
	})
	if err == nil || !strings.Contains(err.Error(), "segment 1") {
		t.Fatalf("err = %v, want a segment 1 failure", err)
	}
	if failed == "" {
		t.Fatal("no step_failed event was emitted")
	}
}

func TestSessionAuditTurnResultRecomputesOutOfRangeScore(t *testing.T) {
	got := sessionAuditTurnResultFrom(3, sessionAuditTurnVerdict{
		Score:            1.5,
		Contradiction:    1,
		InvalidInference: 1,
	}, false)
	want := 1 - 0.15 - 0.18
	if got.Score != want {
		t.Fatalf("score = %v, want %v recomputed from the counts", got.Score, want)
	}
	if got.Issues != 2 {
		t.Fatalf("issues = %d, want 2", got.Issues)
	}
}

func TestDecodeSessionAuditReviewNormalizesVerdict(t *testing.T) {
	got, err := decodeSessionAuditReview(`{"score":0.845,"trend":"unknown","explanation":" 依据 ","issues":[
		{"type":"","turns":[1],"note":"无类型"},
		{"type":"unmet_commitment","turns":[5,2],"note":"承诺未兑现"},
		{"type":"cross_turn_drift","turns":[],"note":"没有轮号"}]}`)
	if err != nil {
		t.Fatalf("decodeSessionAuditReview: %v", err)
	}
	if got.Score != 0.85 || got.Trend != "stable" || got.Explanation != "依据" {
		t.Fatalf("verdict = %+v, want a rounded score, stable trend, trimmed explanation", got)
	}
	if len(got.Issues) != 1 || got.Issues[0].Turns[0] != 2 || got.Issues[0].Turns[1] != 5 {
		t.Fatalf("issues = %+v, want the single typed issue with ascending turns", got.Issues)
	}
}

func TestDecodeSessionAuditReviewRejectsOutOfRangeScore(t *testing.T) {
	if _, err := decodeSessionAuditReview(`{"score":1.4}`); err == nil {
		t.Fatal("out-of-range session score was accepted")
	}
}
