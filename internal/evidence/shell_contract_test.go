package evidence

import (
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/shellparse"
)

func TestBashToolCallUsesNonTerminalInlineInterpreter(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{command: `python3 -c 'open("x","w").write("y")' ; node verify_frontend_logic.js`, want: true},
		{command: `node -e 'console.log(1)' || go test ./...`, want: true},
		{command: `node -e 'console.log(1)' | tee out.txt`, want: true},
		// `&&` short-circuits: a failing interpreter is still the call's exit
		// status, so nothing is hidden and the shape stays allowed.
		{command: `node -e 'console.log(1)' && go test ./...`, want: false},
		{command: `python3 -c 'print(1)'`, want: false},
		{command: `go test ./...`, want: false},
		{command: `node --check app.js`, want: false},
	}
	for _, tt := range tests {
		args, err := json.Marshal(map[string]string{"command": tt.command})
		if err != nil {
			t.Fatal(err)
		}
		if got := BashToolCallUsesNonTerminalInlineInterpreter(args); got != tt.want {
			t.Errorf("%q => %v, want %v", tt.command, got, tt.want)
		}
	}
}

func TestOrdinaryModeShellContractClassifiers(t *testing.T) {
	// deliveryMixed is the broad receipt-integrity classifier Delivery keeps;
	// ordinaryMixed additionally requires that the earlier failure can be hidden.
	cases := []struct {
		command       string
		deliveryMixed bool
		ordinaryMixed bool
		mask          bool
		inline        bool
	}{
		// Arbitrary node scripts are not host-recognized verifiers; the
		// non-terminal inline interpreter rule still blocks this shape.
		{command: `python3 -c 'open("/tmp/x","w").write("x")' ; node verify_frontend_logic.js`, inline: true},
		// `;` lets the verifier's status stand in for the generate step's.
		{command: `go generate ./... ; go test ./...`, deliveryMixed: true, ordinaryMixed: true},
		// `&&` reports the failing step, so ordinary mode has nothing to protect.
		{command: `go generate ./... && go test ./...`, deliveryMixed: true},
		{command: `go build ./... && go test ./...`, deliveryMixed: true},
		{command: `npm install && npm test`, deliveryMixed: true},
		{command: `cargo build && cargo clippy`, deliveryMixed: true},
		// Masked exit is also mixed (echo is not a verifier); agent checks mask first.
		{command: `go test ./...; echo $?`, deliveryMixed: true, ordinaryMixed: true, mask: true},
		{command: `python3 -c 'print(1)'`},
		{command: `go test ./...`},
		{command: `tail -n +1 file | node --check -`},
	}
	for _, tt := range cases {
		args, _ := json.Marshal(map[string]string{"command": tt.command})
		if got := BashToolCallMixesMutationAndVerification(args); got != tt.deliveryMixed {
			t.Errorf("deliveryMixed(%q)=%v want %v", tt.command, got, tt.deliveryMixed)
		}
		if got := BashToolCallMixesMutationAndMaskableVerification(args); got != tt.ordinaryMixed {
			t.Errorf("ordinaryMixed(%q)=%v want %v", tt.command, got, tt.ordinaryMixed)
		}
		if got := BashToolCallMasksVerificationExit(args); got != tt.mask {
			t.Errorf("mask(%q)=%v want %v", tt.command, got, tt.mask)
		}
		if got := BashToolCallUsesNonTerminalInlineInterpreter(args); got != tt.inline {
			t.Errorf("inlineNT(%q)=%v want %v", tt.command, got, tt.inline)
		}
	}
}

// TestOrdinaryModeAllowsShortCircuitBuildAndVerify pins the regression that
// motivated the narrow ordinary-mode classifier: the everyday
// "build, then verify" chain must stay runnable outside Delivery.
func TestOrdinaryModeAllowsShortCircuitBuildAndVerify(t *testing.T) {
	allowed := []string{
		`go build ./... && go test ./...`,
		`npm install && npm test`,
		`pnpm install && pnpm test`,
		`cargo build && cargo test`,
		`make build && make test`,
		`git pull && go test ./...`,
		`mkdir -p out && go test ./...`,
		`go mod tidy && go test ./...`,
	}
	for _, command := range allowed {
		args, _ := json.Marshal(map[string]string{"command": command})
		if BashToolCallMixesMutationAndMaskableVerification(args) {
			t.Errorf("ordinary mode must allow %q: bash reports the failing step's status", command)
		}
		if !BashToolCallMixesMutationAndVerification(args) {
			t.Errorf("delivery mode should still classify %q as mixed", command)
		}
	}
}

// TestSplitHintProposalPassesTheSameContract closes the loop the refusal opens:
// the calls it names are the model's own segments, and each one, on its own,
// must clear every guard that blocked the compound command — otherwise the
// "retry" it proposes would be blocked again.
func TestSplitHintProposalPassesTheSameContract(t *testing.T) {
	compound := `gofmt -w . ; go test ./internal/agent/`
	for _, reason := range []string{"mixed", "mask_exit", "inline_nonterminal"} {
		message := ShellContractPreflightMessage(reason, compound)
		if !strings.Contains(message, "send each as its own call") {
			t.Fatalf("%s: refusal does not name the calls: %q", reason, message)
		}
	}
	segments, split, ok := shellparse.SplitTopLevel(compound)
	if !ok || !split || len(segments) != 2 {
		t.Fatalf("fixture: split=%v segments=%q", split, segments)
	}
	for _, segment := range segments {
		args, _ := json.Marshal(map[string]string{"command": segment})
		switch {
		case BashToolCallMasksVerificationExit(args):
			t.Errorf("proposed call %q would be refused as masked exit", segment)
		case BashToolCallMixesMutationAndMaskableVerification(args):
			t.Errorf("proposed call %q would be refused as mixed", segment)
		case BashToolCallMixesMutationAndVerification(args):
			t.Errorf("proposed call %q would be refused as mixed in delivery mode", segment)
		case BashToolCallUsesNonTerminalInlineInterpreter(args):
			t.Errorf("proposed call %q would be refused as a non-terminal inline interpreter", segment)
		}
	}
}

// TestSplitHintStaysEmptyWhenThereIsNothingToSplit keeps the refusal from
// growing a proposal it cannot back: a single command, unparseable input, and a
// chain too long to render all fall back to the plain recovery text.
func TestSplitHintStaysEmptyWhenThereIsNothingToSplit(t *testing.T) {
	for _, command := range []string{
		`go test ./...`,
		``,
		`echo 'unclosed`,
		`a && b ; c && d ; e`,
	} {
		if hint := ShellContractSplitHint(command); hint != "" {
			t.Errorf("hint(%q) = %q, want none", command, hint)
		}
	}
}
