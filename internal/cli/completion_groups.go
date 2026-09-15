package cli

// Larger subcommand completion groups live here so cliCompletionRootSpec stays
// inside its function-size budget. Entries are unchanged, only relocated.

func agentsCompletionSpec(help cliCompletionFlag) cliCompletionSpec {
	return completionSpec("agents", []cliCompletionFlag{help},
		completionSpec("ls", []cliCompletionFlag{help}),
		completionSpec("up", []cliCompletionFlag{
			completionFlag("--name", cliCompletionStaticValue),
			completionFlag("--session", cliCompletionPathValue),
			completionFlag("--addr", cliCompletionStaticValue), help,
		}),
		completionSpec("down", []cliCompletionFlag{help}),
	)
}

func mcpCompletionSpec(help cliCompletionFlag) cliCompletionSpec {
	return completionSpec("mcp", []cliCompletionFlag{help},
		completionSpecWithAliases("list", []string{"ls"}, []cliCompletionFlag{help}),
		completionSpec("get", []cliCompletionFlag{help}),
		completionSpec("add", []cliCompletionFlag{
			completionFlag("--http --streamable-http --sse --env --header", cliCompletionStaticValue), help,
		}),
		completionSpec("enable", []cliCompletionFlag{help}),
		completionSpec("disable", []cliCompletionFlag{help}),
		completionSpecWithAliases("retry", []string{"connect"}, []cliCompletionFlag{help}),
		completionSpec("update", []cliCompletionFlag{help}),
		completionSpec("import", []cliCompletionFlag{help}),
		completionSpecWithAliases("browse", []string{"search"}, []cliCompletionFlag{
			completionFlag("--limit", cliCompletionStaticValue), completionFlag("--json", cliCompletionNoValue), help,
		}),
		completionSpec("install", []cliCompletionFlag{completionFlag("--as", cliCompletionStaticValue), help}),
		completionSpecWithAliases("remove", []string{"rm"}, []cliCompletionFlag{help}),
	)
}
