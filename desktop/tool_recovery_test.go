package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"reasonix/internal/control"
	"reasonix/internal/provider"
)

func TestToolRecoverySnapshotWithoutControllerIsQuiet(t *testing.T) {
	a := &App{tabs: map[string]*WorkspaceTab{"booting": {ID: "booting", Scope: "project", SessionPath: "/sessions/booting.jsonl"}}}
	view, err := a.GetToolRecoveryForTab("booting")
	if err != nil {
		t.Fatalf("a tab without a controller reported an error: %v", err)
	}
	if view.SessionPath != "/sessions/booting.jsonl" || len(view.Calls) != 0 {
		t.Fatalf("empty view = %+v, want that tab's session path and no calls", view)
	}
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	// The panel reads view.calls.length, so the quiet answer must marshal as [].
	if !strings.Contains(string(body), `"calls":[]`) {
		t.Fatalf("quiet view marshals without a call list: %s", body)
	}
}

func TestRemoteToolRecoveryKeepsIdentityAndNeverReplaysUnknownPost(t *testing.T) {
	posts := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/tool-recovery" || req.Header.Get(expectedSessionPathHeader) != runtimeRemoteTestPath {
			t.Fatalf("unfenced recovery request: %s %v", req.URL, req.Header)
		}
		if req.Method == http.MethodGet {
			return remoteRuntimeTestResponse(req, 200, remoteRuntimeTestJSON(t, control.ToolRecoverySnapshot{SessionPath: runtimeRemoteTestPath, RuntimeEpoch: "epoch", Revision: "revision", Calls: []provider.ToolCallRecord{}})), nil
		}
		posts++
		var body control.ToolRecoveryRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.AttemptID != "attempt" || body.InspectionID != "inspection" || body.RuntimeEpoch != "epoch" || body.Revision != "revision" {
			t.Fatalf("identity changed: %+v", body)
		}
		return nil, errors.New("response lost after commit")
	})}
	a, tab := remoteRuntimeTestApp(client)
	v, err := a.GetToolRecoveryForTab(tab.id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.ResolveToolRecoveryForTab(tab.id, control.ToolRecoveryRequest{SessionPath: v.SessionPath, RuntimeEpoch: v.RuntimeEpoch, Revision: v.Revision, AttemptID: "attempt", InspectionID: "inspection", Action: "confirm"})
	if err == nil || posts != 1 {
		t.Fatalf("unknown post replayed: posts=%d err=%v", posts, err)
	}
}

func TestRemoteToolRecoveryRejectsOtherSessionSnapshot(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return remoteRuntimeTestResponse(req, 200, `{"sessionPath":"/different","runtimeEpoch":"epoch","revision":"revision","calls":[]}`), nil
	})}
	a, tab := remoteRuntimeTestApp(client)
	if _, err := a.GetToolRecoveryForTab(tab.id); err == nil {
		t.Fatal("another session's snapshot was accepted")
	}
}
