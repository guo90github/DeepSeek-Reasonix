package memory

import (
	"context"
	"encoding/json"
	"testing"
)

type fetchRecordingQueue struct {
	queries []string
	facts   []Memory
}

func (q *fetchRecordingQueue) QueueMemory(string) {}

func (q *fetchRecordingQueue) RecordMemoryFetch(query string, facts []Memory) {
	q.queries = append(q.queries, query)
	q.facts = append(q.facts, facts...)
}

type plainQueue struct{ notes []string }

func (q *plainQueue) QueueMemory(note string) { q.notes = append(q.notes, note) }

// The read tool is the only thing that reports a fact the model reached for, so it
// has to name both the ask and what it handed back.
func TestRecallToolReportsWhatItHandedBack(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	if _, err := store.Save(Memory{
		Name: "alpha", Title: "Alpha", Description: "alpha fact",
		Type: TypeProject, Scope: FactScopeProject, Body: "alpha",
	}); err != nil {
		t.Fatal(err)
	}
	queue := &fetchRecordingQueue{}
	ctx := WithQueue(context.Background(), queue)

	out, err := recallTool{store: store}.Execute(ctx, json.RawMessage(`{"operation":"search","query":"alpha"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("search returned nothing to the model")
	}
	if len(queue.queries) != 1 || queue.queries[0] != "alpha" {
		t.Fatalf("queries = %v, want the ask recorded", queue.queries)
	}
	if len(queue.facts) != 1 || queue.facts[0].Name != "alpha" {
		t.Fatalf("facts = %+v, want the hit recorded", queue.facts)
	}
}

// The optional half must stay optional: a queue that cannot take a fetch leaves the
// read working rather than failing it.
func TestRecallToolToleratesAQueueWithoutTheSink(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	if _, err := store.Save(Memory{
		Name: "beta", Title: "Beta", Description: "beta fact",
		Type: TypeProject, Scope: FactScopeProject, Body: "beta",
	}); err != nil {
		t.Fatal(err)
	}
	queue := &plainQueue{}
	ctx := WithQueue(context.Background(), queue)

	out, err := recallTool{store: store}.Execute(ctx, json.RawMessage(`{"operation":"read","name":"beta"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("read returned nothing to the model")
	}
}
