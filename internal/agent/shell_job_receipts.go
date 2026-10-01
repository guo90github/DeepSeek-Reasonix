package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// shellJobCommands remembers which command started which background job. A
// collection only ever reports the job id, so without this map a backgrounded
// check could never be credited to the command the model actually ran.
type shellJobCommands struct {
	mu       sync.Mutex
	commands map[string]string
}

func (s *shellJobCommands) remember(jobID, command string) {
	if jobID == "" || strings.TrimSpace(command) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.commands == nil {
		s.commands = map[string]string{}
	}
	s.commands[jobID] = command
}

func (s *shellJobCommands) lookup(jobID string) (string, bool) {
	if jobID == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	command, ok := s.commands[jobID]
	return command, ok
}

// noteBackgroundShellOutcome ties a shell result to the job behind it: a call
// that starts a job records the command it launched, and a call that collects a
// finished job credits that command with the check it earned.
func (a *Agent) noteBackgroundShellOutcome(call provider.ToolCall, o *toolOutcome) {
	if a == nil || o == nil || o.execution == nil {
		return
	}
	switch {
	case o.execution.State == tool.ShellStateBackgroundStarted:
		if call.Name == "bash" || call.Name == "shell" {
			a.shellJobs.remember(o.execution.JobID, bashCommandFromArgs(json.RawMessage(call.Arguments)))
		}
	case call.Name == "bash_output" || call.Name == "wait":
		a.recordCollectedShellJob(o)
	}
}

// degradeBackgroundStartReceipt stops a started job from reading as a completed
// command. Starting a job proves nothing about the command: it may still fail,
// still be running, or still be about to write. Command text and the success bit
// are both cleared, so neither "ran successfully" nor "ran and failed" matches
// it; collecting the job is what records what actually happened.
func degradeBackgroundStartReceipt(rec *evidence.Receipt, execution *tool.ShellExecution) {
	if rec == nil || execution == nil || execution.State != tool.ShellStateBackgroundStarted {
		return
	}
	rec.Success = false
	rec.Command = ""
}

// recordCollectedShellJob credits a finished background job to the command that
// started it. Only a job that ended in success earns a successful receipt: a
// failed or killed job is recorded as exactly that, and a job that is still
// running is not recorded at all.
func (a *Agent) recordCollectedShellJob(o *toolOutcome) {
	if a == nil || a.task.ledger == nil || o == nil || o.execution == nil || o.errMsg != "" {
		return
	}
	state := o.execution.State
	if state == tool.ShellStateRunning || state == tool.ShellStateBackgroundStarted {
		return
	}
	command, ok := a.shellJobs.lookup(o.execution.JobID)
	if !ok || strings.TrimSpace(command) == "" {
		return
	}
	args, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		return
	}
	succeeded := state == tool.ShellStateCompleted
	if succeeded && a.task.ledger.HasSuccessfulCommand(command) {
		return
	}
	rec := evidence.ReceiptFromToolCall("bash", args, succeeded, false)
	rec = a.task.ledger.Record(rec)
	if !succeeded {
		return
	}
	o.output += fmt.Sprintf("\ncollected as receipt %s for `%s` — the check ran; cite it to sign this off.", rec.ID, command)
}
