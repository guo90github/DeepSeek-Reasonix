package config

import "time"

// AskTimeout resolves the [agent] ask_timeout_minutes knob for the controller: a
// zero value keeps control's built-in wait, a negative one waits for the user
// instead of answering for them, and a positive one is that many minutes. The
// duration passed through is seconds-accurate even for fractional minutes.
func (c *Config) AskTimeout() time.Duration {
	if c == nil {
		return 0
	}
	switch minutes := c.Agent.AskTimeoutMinutes; {
	case minutes < 0:
		return -1
	case minutes == 0:
		return 0
	default:
		return time.Duration(minutes * float64(time.Minute))
	}
}
