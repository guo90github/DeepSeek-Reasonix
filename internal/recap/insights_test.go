package recap

import (
	"context"
	"testing"
	"time"
)

// An insight is the only claim a single session cannot make: the same conclusion
// reached somewhere else, on its own. What makes it worth showing is the second
// project, so that is what this pins — including the case that must stay out.
func TestInsightsListConclusionsMoreThanOneProjectReached(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	now := time.Unix(1800000000, 0)
	alpha := `C:\state\projects\alpha`
	beta := `C:\state\projects\beta`
	gamma := `C:\state\projects\gamma`

	shared := Entry{Kind: KindRefuted, Body: "只取分支统计未提交数会漏掉 CJK 路径",
		Evidence: "desktop/gitstats.go desktop/gitstats_test.go"}
	sharedElsewhere := Entry{Kind: KindRefuted, Body: "统计未提交数只取分支会漏掉 CJK 路径",
		Evidence: "desktop/gitstats.go desktop/gitstats_test.go"}
	lonely := Entry{Kind: KindRootCause, Body: "面板恒显示 0 个文件：从不填充 Files",
		Evidence: "desktop/workspace_git_branches.go"}

	put := func(bucket, name string, at time.Time, entries ...Entry) {
		t.Helper()
		if err := store.Put(ctx, Record{Path: bucket + `\sessions\` + name, Fingerprint: "f",
			PromptVersion: PromptVersion, GeneratedAt: at, Entries: entries}); err != nil {
			t.Fatalf("put %s: %v", name, err)
		}
	}
	put(alpha, "a.jsonl", now.Add(-2*time.Hour), shared, lonely)
	put(beta, "b.jsonl", now.Add(-1*time.Hour), sharedElsewhere)
	put(gamma, "c.jsonl", now.Add(-30*time.Minute), sharedElsewhere)

	got, err := store.Insights(ctx, time.Time{}, 10)
	if err != nil {
		t.Fatalf("insights: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("insights = %d, want only the conclusion three projects reached: %+v", len(got), got)
	}
	if got[0].Kind != KindRefuted || len(got[0].Projects) != 3 {
		t.Fatalf("insight = %+v, want the refuted conclusion from three projects", got[0])
	}
	want := []string{"alpha", "beta", "gamma"}
	for i, label := range want {
		if got[0].Projects[i] != label {
			t.Fatalf("projects = %v, want %v", got[0].Projects, want)
		}
	}
	if !got[0].SeenAt.Equal(now.Add(-30 * time.Minute)) {
		t.Fatalf("seen at = %v, want the newest copy", got[0].SeenAt)
	}

	// The period is what makes it a report rather than a dump.
	if recent, err := store.Insights(ctx, now.Add(-time.Hour), 10); err != nil || len(recent) != 1 {
		t.Fatalf("insights inside the last hour = %+v (%v), want the same conclusion", recent, err)
	}
	if older, err := store.Insights(ctx, now.Add(-15*time.Minute), 10); err != nil || len(older) != 0 {
		t.Fatalf("insights inside the last 15m = %+v (%v), want none", older, err)
	}

	// A conclusion that keeps coming back inside one project earns the report too:
	// "this project reached it twice" is the second kind of evidence, and it needs
	// no other project to hold.
	sameBucket := Entry{Kind: KindRootCause, Body: "面板恒显示 0 个文件：从不填充 Files",
		Evidence: "desktop/workspace_git_branches.go"}
	put(alpha, "d.jsonl", now.Add(-10*time.Minute), sameBucket)
	repeat, err := store.Insights(ctx, time.Time{}, 10)
	if err != nil {
		t.Fatalf("insights after the repeat: %v", err)
	}
	found := false
	for _, insight := range repeat {
		if insight.Body == sameBucket.Body {
			found = true
			if insight.Occurrences != 2 || len(insight.Projects) != 1 {
				t.Fatalf("a repeat inside one project must report twice in one project: %+v", insight)
			}
		}
	}
	if !found {
		t.Fatalf("a conclusion two records of one project reached must be reported: %+v", repeat)
	}

}
