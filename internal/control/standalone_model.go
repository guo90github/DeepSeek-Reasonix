package control

import (
	"fmt"
	"strings"

	"reasonix/internal/provider"
)

// resolveStandaloneModel builds the provider for a feature with its own
// dedicated model (prompt optimization, reasoning audit). It is deliberately
// stateless: every input comes from parameters, so no two features can share or
// override each other's configuration. An empty modelRef or a missing resolver
// disables the feature rather than falling back to the session model.
func (c *Controller) resolveStandaloneModel(feature, modelRef string, resolver func(string) (provider.Provider, error)) (provider.Provider, error) {
	if strings.TrimSpace(modelRef) == "" {
		return nil, fmt.Errorf("%s：模型未配置，请在设置中选择", feature)
	}
	if resolver == nil {
		return nil, fmt.Errorf("%s：resolver 未就绪", feature)
	}
	p, err := resolver(modelRef)
	if err != nil {
		return nil, fmt.Errorf("%s：%w", feature, err)
	}
	return p, nil
}
