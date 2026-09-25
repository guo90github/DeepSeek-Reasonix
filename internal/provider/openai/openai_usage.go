package openai

import (
	"reasonix/internal/provider"
)

// mergeUsage folds token counters. countRequests is false for multiple usage
// chunks from one HTTP stream (keep its request count), and true when combining
// distinct prefix-continuation requests (sum their request counts).
func mergeUsage(total, next *provider.Usage, countRequests bool) *provider.Usage {
	if next == nil {
		return total
	}
	if total == nil {
		clone := *next
		return &clone
	}
	totalRequests := usageRequestCount(total)
	nextRequests := usageRequestCount(next)
	total.PromptTokens += next.PromptTokens
	total.CompletionTokens += next.CompletionTokens
	total.TotalTokens += next.TotalTokens
	total.CacheHitTokens += next.CacheHitTokens
	total.CacheMissTokens += next.CacheMissTokens
	total.CacheWriteTokens += next.CacheWriteTokens
	total.CacheWriteBilledTokens += next.CacheWriteBilledTokens
	total.ReasoningTokens += next.ReasoningTokens
	if countRequests {
		total.RequestCount = totalRequests + nextRequests
	} else if nextRequests > totalRequests {
		total.RequestCount = nextRequests
	} else {
		total.RequestCount = totalRequests
	}
	total.FinishReason = next.FinishReason
	return total
}

// normaliseUsage folds the cache shapes used by OpenAI-compatible providers into
// a single Usage. DeepSeek reports prompt_cache_{hit,miss}_tokens at the top of
// usage; OpenAI and MiMo put cache hits under prompt_tokens_details; some
// compatible gateways return Anthropic-style input/cache counters instead.
// Reasoning tokens land in completion_tokens_details on thinking-mode models.
func normaliseUsage(u *wireUsage) *provider.Usage {
	prompt := u.PromptTokens
	anthropicPrompt := prompt == 0 &&
		(u.InputTokens != 0 || u.CacheCreationInputTokens != 0 || u.CacheReadInputTokens != 0)
	if anthropicPrompt {
		// Anthropic-style input_tokens excludes both cache reads and cache
		// writes, while Reasonix PromptTokens represents the complete input.
		prompt = u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	}
	completion := u.CompletionTokens
	if completion == 0 {
		completion = u.OutputTokens
	}
	total := u.TotalTokens
	if total == 0 && (prompt != 0 || completion != 0) {
		total = prompt + completion
	}

	hit := u.PromptCacheHitTokens
	miss := u.PromptCacheMissTokens
	if hit == 0 && u.PromptTokensDetails != nil {
		hit = u.PromptTokensDetails.CachedTokens
	}
	if hit == 0 {
		hit = u.CacheReadInputTokens
	}
	if miss == 0 {
		switch {
		case anthropicPrompt:
			// Cache writes are still uncached input for Reasonix pricing and
			// cache-ratio accounting.
			miss = u.InputTokens + u.CacheCreationInputTokens
		case hit > 0 && prompt > hit:
			miss = prompt - hit
		}
	}
	reasoning := 0
	if u.CompletionTokensDetails != nil {
		reasoning = u.CompletionTokensDetails.ReasoningTokens
	}
	return &provider.Usage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
		CacheHitTokens:   hit,
		CacheMissTokens:  miss,
		ReasoningTokens:  reasoning,
	}
}
