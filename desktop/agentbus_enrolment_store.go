package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/fileutil"
)

// Enrolment lives in the controller, which is memory: a restart — crash, watchdog restore,
// version switch — would leave every session off the board, so it stops advancing exactly when
// it was restored (real machine, 2026-10-03). Remembered per session path, re-applied on build.

type agentBusEnrolment struct {
	Dir         string `json:"dir"`
	Participant string `json:"participant,omitempty"`
	At          string `json:"at,omitempty"`
}

type agentBusEnrolments struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Sessions      map[string]agentBusEnrolment `json:"sessions"`
}

const agentBusEnrolmentSchemaVersion = 1

var agentBusEnrolmentMu sync.Mutex

func agentBusEnrolmentPath() string {
	root := strings.TrimSpace(config.MemoryUserDir())
	if root == "" {
		return ""
	}
	return filepath.Join(root, "agentbus-enrolments.json")
}

func readAgentBusEnrolments() agentBusEnrolments {
	store := agentBusEnrolments{SchemaVersion: agentBusEnrolmentSchemaVersion, Sessions: map[string]agentBusEnrolment{}}
	path := agentBusEnrolmentPath()
	if path == "" {
		return store
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return store
	}
	var loaded agentBusEnrolments
	if err := json.Unmarshal(body, &loaded); err != nil {
		return store
	}
	for session, record := range loaded.Sessions {
		if strings.TrimSpace(record.Dir) == "" {
			continue
		}
		store.Sessions[session] = record
	}
	return store
}

func writeAgentBusEnrolments(store agentBusEnrolments) error {
	path := agentBusEnrolmentPath()
	if path == "" {
		return fmt.Errorf("desktop: no state home to remember the board in")
	}
	if store.Sessions == nil {
		store.Sessions = map[string]agentBusEnrolment{}
	}
	store.SchemaVersion = agentBusEnrolmentSchemaVersion
	body, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, append(body, '\n'), 0o600)
}

// rememberAgentBusEnrolment records the session's board so a later host start can put it
// back. A session path is required: without one there is no way to recognise the session
// again, and the participant id can always be re-resolved from the board.
func rememberAgentBusEnrolment(sessionPath, dir, participant string) error {
	sessionPath = strings.TrimSpace(sessionPath)
	dir = strings.TrimSpace(dir)
	if sessionPath == "" || dir == "" {
		return nil
	}
	agentBusEnrolmentMu.Lock()
	defer agentBusEnrolmentMu.Unlock()
	store := readAgentBusEnrolments()
	if existing, ok := store.Sessions[sessionPath]; ok && existing.Dir == dir && existing.Participant == participant {
		return nil
	}
	store.Sessions[sessionPath] = agentBusEnrolment{
		Dir: dir, Participant: strings.TrimSpace(participant), At: time.Now().UTC().Format(time.RFC3339),
	}
	return writeAgentBusEnrolments(store)
}

func forgetAgentBusEnrolment(sessionPath string) error {
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" {
		return nil
	}
	agentBusEnrolmentMu.Lock()
	defer agentBusEnrolmentMu.Unlock()
	store := readAgentBusEnrolments()
	if _, ok := store.Sessions[sessionPath]; !ok {
		return nil
	}
	delete(store.Sessions, sessionPath)
	return writeAgentBusEnrolments(store)
}

// rememberedAgentBusEnrolment reports the board this session joined before.
func rememberedAgentBusEnrolment(sessionPath string) (agentBusEnrolment, bool) {
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" {
		return agentBusEnrolment{}, false
	}
	agentBusEnrolmentMu.Lock()
	defer agentBusEnrolmentMu.Unlock()
	record, ok := readAgentBusEnrolments().Sessions[sessionPath]
	if !ok || strings.TrimSpace(record.Dir) == "" {
		return agentBusEnrolment{}, false
	}
	return record, true
}
