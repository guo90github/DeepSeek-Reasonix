package tool

// CapabilityNames reports the optional interfaces one tool satisfies, in a stable order. It lives
// here, not in a test: the contract, the registry snapshot and the documentation all have to read
// the same answer, or a capability shows up in one place and not the other (I1, 2026-10-05).
func CapabilityNames(t Tool) []string {
	var x any = t
	var out []string
	add := func(name string, ok bool) {
		if ok {
			out = append(out, name)
		}
	}
	_, batch := x.(BatchClassifier)
	add("BatchClassifier", batch)
	_, contextual := x.(ContextualTool)
	add("ContextualTool", contextual)
	_, preview := x.(Previewer)
	add("Previewer", preview)
	_, image := x.(ImageTool)
	add("ImageTool", image)
	_, planMode := x.(PlanModeClassifier)
	add("PlanModeClassifier", planMode)
	_, hostMutation := x.(ReadOnlyExecutionHostMutation)
	add("ReadOnlyExecutionHostMutation", hostMutation)
	_, blockReason := x.(ReadOnlyExecutionBlockReason)
	add("ReadOnlyExecutionBlockReason", blockReason)
	_, mcpMeta := x.(MCPMetadata)
	add("MCPMetadata", mcpMeta)
	_, mcpVisible := x.(MCPVisibleMetadata)
	add("MCPVisibleMetadata", mcpVisible)
	_, mcpPackage := x.(MCPPackageMetadata)
	add("MCPPackageMetadata", mcpPackage)
	_, mcpAnnotations := x.(MCPAnnotations)
	add("MCPAnnotations", mcpAnnotations)
	_, mcpAuth := x.(MCPServerAuthorization)
	add("MCPServerAuthorization", mcpAuth)
	_, snip := x.(SnipHinter)
	add("SnipHinter", snip)
	return out
}
