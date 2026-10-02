package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/agentbus"
)

// agentBusTalkMaxLines bounds how much talk one turn may carry, mirroring the
// board view's line bound: an unbounded digest is a cost bomb.
const agentBusTalkMaxLines = 50

// SetAgentBusTalkLimits sets the boundaries this session's talk obeys. Zero
// values leave a boundary off, which is the default: the kernel invents no
// ceilings on the operator's behalf.
func (c *Controller) SetAgentBusTalkLimits(limits agentbus.TalkLimits) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentBus != nil {
		c.agentBus.limits = limits
	}
}

// Say publishes free talk on a topic. Free talk promises nothing; it reaches
// another participant only when it names them.
func (c *Controller) AgentBusSay(ctx context.Context, topic, text string, mentions []string) (uint64, error) {
	line, err := c.appendTalk(ctx, agentbus.TalkLine{
		Topic: topic, Kind: agentbus.TalkSay, Text: text, Mentions: mentions,
	})
	return line.Seq, err
}

// Ask opens a bounded question chain and returns the correlation the answer must
// cite. The chain's budget, TTL and hop count are enforced by the log, so a
// question that outlives its bounds is refused rather than quietly piling up.
func (c *Controller) AgentBusAsk(ctx context.Context, topic, to, text string) (string, error) {
	line, err := c.appendTalk(ctx, agentbus.TalkLine{
		Topic: topic, Kind: agentbus.TalkAsk, To: to, Text: text,
	})
	if err != nil {
		return "", err
	}
	return line.Correlation, nil
}

// Answer closes a chain another participant opened. The recipient is explicit
// because the answering side already knows who asked: the ask line it received
// carries that.
func (c *Controller) AgentBusAnswer(ctx context.Context, correlation, topic, to, text string) (uint64, error) {
	correlation = strings.TrimSpace(correlation)
	if correlation == "" {
		return 0, fmt.Errorf("control: answer needs the correlation it replies to")
	}
	line, err := c.appendTalk(ctx, agentbus.TalkLine{
		Topic: topic, Kind: agentbus.TalkAnswer, To: to, Correlation: correlation, Text: text,
	})
	return line.Seq, err
}

// PostAgentBusResult publishes the structured reply for one correlation.
func (c *Controller) PostAgentBusResult(result agentbus.Result) error {
	dir, err := c.agentBusDir()
	if err != nil {
		return err
	}
	return agentbus.WriteResult(dir, result)
}

// ReadAgentBusResult reads the structured reply for one correlation. A missing
// envelope is a false, not an error: nobody has replied yet.
func (c *Controller) ReadAgentBusResult(correlation string) (agentbus.Result, bool, error) {
	dir, err := c.agentBusDir()
	if err != nil {
		return agentbus.Result{}, false, err
	}
	return agentbus.ReadResult(dir, correlation)
}

// agentBusTalkBlock renders what this participant is addressed by — and nothing
// else — plus a count of the speech that named nobody, then advances the cursor.
// Free talk is not context: it enters an agent's turn only when it names them
// (AGENT_BUS §3.2).
func (c *Controller) agentBusTalkBlock() string {
	bus, st, err := c.agentBusTalkSnapshot()
	if err != nil {
		return ""
	}
	participant := bus.participantID(c)
	cursor := bus.currentTalkCursor()
	names := st.TopicNames()

	var body strings.Builder
	var hidden, lines int
	var delivered uint64
	truncated := false
	for _, name := range names {
		named, rest := agentbus.Digest(st, name, participant, cursor)
		hidden += rest
		for _, line := range named {
			if line.Seq > delivered {
				delivered = line.Seq
			}
			if lines >= agentBusTalkMaxLines {
				truncated = true
				continue
			}
			lines++
			fmt.Fprintf(&body, "line seq=%d topic=%s kind=%s from=%s to=%s correlation=%s text=%q\n",
				line.Seq, line.Topic, line.Kind, line.From, line.To, line.Correlation, line.Text)
		}
	}
	if lines == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "agentbus talk schema=agentbus-talk/1 board=%s participant=%s\n",
		filepath.Base(bus.dir), participant)
	fmt.Fprintf(&b, "state topics=%d lines=%d delivered_to=%d hidden=%d truncated=%t\n",
		len(names), lines, delivered, hidden, truncated)
	b.WriteString(body.String())
	bus.advanceTalk(delivered)
	return b.String()
}

func (c *Controller) agentBusTalkSnapshot() (*agentBusState, *agentbus.TalkState, error) {
	bus, err := c.agentBusForTalk()
	if err != nil {
		return nil, nil, err
	}
	log, err := agentbus.OpenTalkLog(bus.dir)
	if err != nil {
		return nil, nil, err
	}
	st, _, err := log.Read()
	if err != nil {
		return nil, nil, err
	}
	return bus, st, nil
}

// appendTalk publishes one line for this participant. A lapsed topic is closed on
// the way in: the closure is a record written by whoever speaks next, never a
// decision made while reading.
func (c *Controller) appendTalk(ctx context.Context, line agentbus.TalkLine) (agentbus.TalkLine, error) {
	bus, err := c.agentBusForTalk()
	if err != nil {
		return agentbus.TalkLine{}, err
	}
	line.From = bus.participantID(c)
	line.To = strings.TrimSpace(line.To)
	if line.At.IsZero() {
		line.At = time.Now().UTC()
	}
	if line.Kind == agentbus.TalkAsk && line.Correlation == "" {
		line.Correlation = talkCorrelation(line.From, line.Topic, line.To, line.Text)
	}
	log, err := agentbus.OpenTalkLog(bus.dir)
	if err != nil {
		return agentbus.TalkLine{}, err
	}
	if closed, err := log.CloseLapsed(ctx, line.Topic, line.At, bus.limits); err != nil {
		slog.Warn("controller: close lapsed agentbus topic", "topic", line.Topic, "err", err)
	} else if closed {
		slog.Debug("controller: closed lapsed agentbus topic", "topic", line.Topic)
	}
	return log.Append(ctx, line, bus.limits)
}

// talkCorrelation derives a chain id from what was asked, never from when: a
// retried question rejoins its own chain instead of opening a second one.
func talkCorrelation(participant, topic, to, text string) string {
	sum := sha256.Sum256([]byte(participant + "\x00" + topic + "\x00" + to + "\x00" + text))
	return "ask-" + hex.EncodeToString(sum[:8])
}

func (c *Controller) agentBusDir() (string, error) {
	c.mu.Lock()
	bus := c.agentBus
	c.mu.Unlock()
	if bus == nil {
		return "", errAgentBusUnwired
	}
	return bus.dir, nil
}

// agentBusForTalk resolves the state required to speak: enrolled, and holding an
// identity. A session with neither has no standing to interrupt anyone.
func (c *Controller) agentBusForTalk() (*agentBusState, error) {
	c.mu.Lock()
	state := c.agentBus
	c.mu.Unlock()
	if state == nil || state.participantID(c) == "" {
		return nil, errAgentBusUnwired
	}
	return state, nil
}

func (b *agentBusState) currentTalkCursor() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cursors.talk
}

func (b *agentBusState) advanceTalk(seq uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if seq > b.cursors.talk {
		b.cursors.talk = seq
	}
}
