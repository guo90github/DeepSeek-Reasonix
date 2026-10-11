package agent

import (
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// parallelismSample is what the turn readout says about concurrency: the widest
// batch the host dispatched, and whether any of them handed work to another
// worker. Together they decide whether a long turn had a cheaper shape.
type parallelismSample struct {
	maxCallsPerRound int
	delegated        bool
}

func (p *parallelismSample) observe(calls []provider.ToolCall) {
	if len(calls) > p.maxCallsPerRound {
		p.maxCallsPerRound = len(calls)
	}
	for _, call := range calls {
		if isDelegationCall(call.Name) {
			p.delegated = true
			return
		}
	}
}

// isDelegationCall names the tools that hand work to another worker: a turn that
// never reached for one is serial by the model's choice, not by the host's.
func isDelegationCall(name string) bool {
	switch name {
	case tool.HostTask, tool.HostReadOnlyTask, tool.HostParallelTasks, tool.HostFleet, tool.HostRunSkill:
		return true
	}
	return false
}
