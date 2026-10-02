package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/fileutil"
)

// The desktop's own tab file cannot hold the Goal contract: it is rewritten from the live
// controller, which reports no goal once the goal is stopped, so every save after a stop
// erased the contract and an unattended session had nothing to resume (found on a real
// machine, 2026-10-03). The contract is kept here instead — written the moment the desktop
// sees it, never rewritten from a stopped goal — so a crash, a stop or a version switch
// cannot take it away. Dropping it is a deliberate act (see forgetUnattendedGoalContract).

type unattendedGoalContracts struct {
	SchemaVersion int                                     `json:"schemaVersion"`
	Sessions      map[string]unattendedGoalContractRecord `json:"sessions"`
}

type unattendedGoalContractRecord struct {
	Goal string `json:"goal"`
	At   string `json:"at,omitempty"`
}

const unattendedGoalContractSchemaVersion = 1

var unattendedGoalContractMu sync.Mutex

func unattendedGoalContractPath() string {
	root := strings.TrimSpace(config.MemoryUserDir())
	if root == "" {
		return ""
	}
	return filepath.Join(root, "unattended-contracts.json")
}

func readUnattendedGoalContracts() unattendedGoalContracts {
	store := unattendedGoalContracts{
		SchemaVersion: unattendedGoalContractSchemaVersion,
		Sessions:      map[string]unattendedGoalContractRecord{},
	}
	path := unattendedGoalContractPath()
	if path == "" {
		return store
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return store
	}
	var loaded unattendedGoalContracts
	if err := json.Unmarshal(body, &loaded); err != nil {
		return store
	}
	for session, record := range loaded.Sessions {
		if strings.TrimSpace(record.Goal) == "" {
			continue
		}
		store.Sessions[session] = record
	}
	return store
}

func writeUnattendedGoalContracts(store unattendedGoalContracts) error {
	path := unattendedGoalContractPath()
	if path == "" {
		return nil
	}
	store.SchemaVersion = unattendedGoalContractSchemaVersion
	if store.Sessions == nil {
		store.Sessions = map[string]unattendedGoalContractRecord{}
	}
	body, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, append(body, '\n'), 0o600)
}

// rememberUnattendedGoalContract records the contract the desktop just saw. It is called
// wherever a goal text is still visible — loading a tab, resolving the contract — because
// after the next save the tab file no longer carries it.
func rememberUnattendedGoalContract(sessionPath, goal string) {
	sessionPath = strings.TrimSpace(sessionPath)
	goal = strings.TrimSpace(goal)
	if sessionPath == "" || goal == "" {
		return
	}
	unattendedGoalContractMu.Lock()
	defer unattendedGoalContractMu.Unlock()
	store := readUnattendedGoalContracts()
	if existing, ok := store.Sessions[sessionPath]; ok && existing.Goal == goal {
		return
	}
	store.Sessions[sessionPath] = unattendedGoalContractRecord{
		Goal: goal, At: time.Now().UTC().Format(time.RFC3339),
	}
	_ = writeUnattendedGoalContracts(store)
}

func rememberedUnattendedGoalContract(sessionPath string) string {
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" {
		return ""
	}
	unattendedGoalContractMu.Lock()
	defer unattendedGoalContractMu.Unlock()
	return strings.TrimSpace(readUnattendedGoalContracts().Sessions[sessionPath].Goal)
}
