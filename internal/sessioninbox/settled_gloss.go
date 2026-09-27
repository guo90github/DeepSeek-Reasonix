package sessioninbox

// SettledGloss is the one place the human wording of a settled disposition is
// declared. The desktop copy cannot import this, so a test compares the two
// verbatim: one ending must not end up with two names on two sides.
var SettledGloss = map[string]string{
	"acknowledged": "跑完并确认",
	"discarded":    "被丢弃",
	"deleted":      "被删掉",
}
