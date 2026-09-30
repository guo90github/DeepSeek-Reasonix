package recap

import (
	"context"
	"errors"
	"strings"
	"time"

	"reasonix/internal/boundedllm"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// ComposeOptions wires the two calls a person triggers from the recap page: the
// memory rewrite and the playbook. They share the lane's discipline — their own
// usage source, a timeout, no ceiling of their own — but not its lane: a preview
// answers one click.
type ComposeOptions struct {
	PromptDir string
	Timeout   time.Duration
	Sink      event.Sink
}

// ComposeResult is what the preview shows: the text, which rules produced it, and
// why the built-in rules were used when a file could not be.
type ComposeResult struct {
	Text      string
	PromptTag string
	Model     string
	Note      string
}

// errEmptyCompose marks a model that answered nothing: the caller shows the reason
// and falls back to what it can do without a model.
var errEmptyCompose = errors.New("the model returned nothing")

// composeTimeout bounds a preview. Reading a preview is a person waiting, so it is
// shorter than a background recap.
const composeTimeout = 90 * time.Second

// ComposeMemory rewrites one note into the sentence that will be stored as memory.
func ComposeMemory(ctx context.Context, prov provider.Provider, ref, evidence string, opts ComposeOptions) (ComposeResult, error) {
	return compose(ctx, prov, ref, evidence, LoadMemoryPromptOverride(opts.PromptDir), opts)
}

// ComposeSkill writes a playbook from the notes of one topic.
func ComposeSkill(ctx context.Context, prov provider.Provider, ref, evidence string, opts ComposeOptions) (ComposeResult, error) {
	return compose(ctx, prov, ref, evidence, LoadSkillPromptOverride(opts.PromptDir), opts)
}

func compose(ctx context.Context, prov provider.Provider, ref, evidence string, source PromptSource, opts ComposeOptions) (ComposeResult, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = composeTimeout
	}
	// No ceiling of ours: a preview a person asked for must not come back empty
	// because the model spent a completion budget thinking. The timeout, and the
	// request guard sized to what is actually sent, are what bound this call.
	text, err := boundedllm.Call(ctx, boundedllm.Config{
		Provider:       prov,
		ModelRef:       ref,
		Sink:           opts.Sink,
		UsageSource:    event.UsageSourceSessionRecap,
		Timeout:        timeout,
		MaxTokens:      -1,
		MaxOutputBytes: -1,
		MaxSystemBytes: len(source.Text) + 1024,
		MaxTotalBytes:  len(source.Text) + len(evidence) + 4096,
		EffortOverride: provider.PreferredReasoning(prov, "low"),
	}, source.Text, evidence)
	result := ComposeResult{PromptTag: source.Tag, Model: ref, Note: source.Note}
	if err != nil {
		return result, err
	}
	result.Text = strings.TrimSpace(text)
	if result.Text == "" {
		return result, errEmptyCompose
	}
	return result, nil
}
