package browser

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

// waitingExecutor adds the optional Waiter capability to the fake.
type waitingExecutor struct {
	*fakeExecutor
	wait WaitResult
	reqs []WaitRequest
}

func (w *waitingExecutor) Wait(_ context.Context, req WaitRequest) (WaitResult, error) {
	w.reqs = append(w.reqs, req)
	return w.wait, nil
}

func TestWaitReportsTheConditionItSaw(t *testing.T) {
	exec := &waitingExecutor{fakeExecutor: &fakeExecutor{}, wait: WaitResult{Found: true, Elapsed: 420 * time.Millisecond}}
	out, err := run(t, exec, "browser_wait", `{"tabId":"t1","text":"已加载"}`)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !strings.Contains(out, `waited 420ms for text "已加载"`) {
		t.Fatalf("wait out = %q", out)
	}
	want := []WaitRequest{{TabID: "t1", Text: "已加载", Timeout: 10 * time.Second}}
	if !reflect.DeepEqual(exec.reqs, want) {
		t.Fatalf("wait requests = %+v, want %+v", exec.reqs, want)
	}
}

func TestWaitPassesTheTimeoutAndReportsATimeout(t *testing.T) {
	exec := &waitingExecutor{fakeExecutor: &fakeExecutor{}, wait: WaitResult{Found: false, Elapsed: 3 * time.Second}}
	out, err := run(t, exec, "browser_wait", `{"tabId":"t1","seconds":3}`)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !strings.Contains(out, "timeout after 3000ms waiting for the tab to stop loading") {
		t.Fatalf("wait out = %q", out)
	}
	if len(exec.reqs) != 1 || exec.reqs[0].Timeout != 3*time.Second || exec.reqs[0].Text != "" {
		t.Fatalf("wait request = %+v", exec.reqs)
	}
}

func TestWaitGoneReadsAsDisappearance(t *testing.T) {
	exec := &waitingExecutor{fakeExecutor: &fakeExecutor{}, wait: WaitResult{Found: true, Elapsed: 150 * time.Millisecond}}
	out, err := run(t, exec, "browser_wait", `{"tabId":"t1","text":"loading","gone":true}`)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !strings.Contains(out, `waited 150ms for text "loading" to disappear`) {
		t.Fatalf("wait out = %q", out)
	}
}

func TestWaitRejectsAnOutOfRangeTimeout(t *testing.T) {
	exec := &waitingExecutor{fakeExecutor: &fakeExecutor{}, wait: WaitResult{Found: true}}
	_, err := run(t, exec, "browser_wait", `{"tabId":"t1","seconds":61}`)
	if err == nil || !strings.Contains(err.Error(), "between 0 and 60") {
		t.Fatalf("err = %v", err)
	}
	if len(exec.reqs) != 0 {
		t.Fatalf("an out-of-range timeout must not reach the host: %+v", exec.reqs)
	}
}

func TestWaitIsUnavailableWithoutTheCapability(t *testing.T) {
	out, err := run(t, &fakeExecutor{}, "browser_wait", `{"tabId":"t1"}`)
	if err == nil || !strings.Contains(err.Error(), "waiting is unavailable") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}
