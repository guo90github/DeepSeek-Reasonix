package boot

import (
	"fmt"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/extension/providerext"
	"reasonix/internal/guardian"
	"reasonix/internal/netclient"
	"reasonix/internal/provider"
	"reasonix/internal/recovery"
	"reasonix/internal/tool"
)

// configureRecoveryReviewer wires the reviewer named by recovery_model. Empty
// leaves rule-only recovery; a configured but unusable model is a configuration
// error rather than a silent downgrade.
func configureRecoveryReviewer(cfg *config.Config, ctrlOpts *control.Options, extensionResolver provider.Resolver, proxySpec netclient.ProxySpec, sink event.Sink) error {
	// Recovery reviewer is explicit: empty recovery_model leaves rule-only
	// recovery. A configured but unusable model is a configuration error.
	if recoveryModel := strings.TrimSpace(cfg.Agent.RecoveryModel); recoveryModel != "" {
		if extensionResolver != nil && providerext.PluginRefOwner(recoveryModel) != "" {
			re, ok := resolveOptionalEntry(extensionResolver, cfg, recoveryModel)
			if !ok {
				return fmt.Errorf("recovery_model %q is not a configured provider", recoveryModel)
			}
			rProv, err := extensionResolver.Resolve(provider.Selection{Ref: modelRefFromEntry(re)})
			if err != nil {
				return fmt.Errorf("recovery_model %q: %w", recoveryModel, err)
			}
			ctrlOpts.RecoveryReviewer = recovery.NewSessionWithSink(rProv, re.Price, modelRefFromEntry(re), sink)
		} else {
			re, ok := cfg.ResolveModel(recoveryModel)
			if !ok {
				return fmt.Errorf("recovery_model %q is not a configured provider", recoveryModel)
			}
			rProv, err := NewProviderWithProxy(re, proxySpec)
			if err != nil {
				return fmt.Errorf("recovery_model %q: %w", recoveryModel, err)
			}
			ctrlOpts.RecoveryReviewer = recovery.NewSessionWithSink(rProv, re.Price, modelRefFromEntry(re), sink)
		}
	}
	return nil
}

// configureGuardianReviewer spawns the LLM safety reviewer named by
// guardian_model: it auto-allows safe Ask decisions and annotates risky ones
// before they reach the human approval prompt.
func configureGuardianReviewer(cfg *config.Config, ctrlOpts *control.Options, effectiveResolver provider.Resolver, proxySpec netclient.ProxySpec, reg *tool.Registry, sink event.Sink) error {
	// Guardian: when guardian_model is configured, spawn an LLM safety reviewer
	// that can auto-allow safe Ask decisions and annotate risky ones before
	// escalating to the human approval prompt.
	if guardianModel := cfg.Agent.GuardianModel; guardianModel != "" {
		ge, ok := resolveOptionalEntry(effectiveResolver, cfg, guardianModel)
		if !ok {
			return fmt.Errorf("guardian_model %q is not a configured provider", guardianModel)
		}
		pProv, err := resolveProvider(effectiveResolver, cfg, proxySpec, provider.Selection{Ref: modelRefFromEntry(ge)})
		if err != nil {
			return fmt.Errorf("guardian_model %q: %w", guardianModel, err)
		}
		guardianReg := agent.FilterReadOnlyRegistry(reg, agent.SubagentMetaTools()...)
		ctrlOpts.Guardian = guardian.NewSession(pProv, guardianReg, guardian.PolicyPrompt(), modelRefFromEntry(ge), cfg.Agent.GuardianTemperature, ge.Price, sink)
		sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Text: fmt.Sprintf("guardian enabled · model=%s", ge.Model)})
	}
	return nil
}
