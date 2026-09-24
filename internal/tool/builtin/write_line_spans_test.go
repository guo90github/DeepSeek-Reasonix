package builtin

import (
	"slices"
	"testing"

	"reasonix/internal/evidence"
)

func TestWriteLineSpansAccountForEveryAppliedHunk(t *testing.T) {
	cases := []struct {
		name    string
		oldText string
		newText string
		want    []evidence.WriteLineSpan
	}{
		{
			name:    "replace one line in place",
			oldText: "a\nb\nc\n",
			newText: "a\nB\nc\n",
			want:    []evidence.WriteLineSpan{{FirstLine: 2, LastLine: 2, Delta: 0}},
		},
		{
			name:    "insert a line below",
			oldText: "a\nb\n",
			newText: "a\nX\nb\n",
			want:    []evidence.WriteLineSpan{{FirstLine: 2, LastLine: 2, Delta: 1}},
		},
		{
			name:    "insert a line above",
			oldText: "a\nb\n",
			newText: "X\na\nb\n",
			want:    []evidence.WriteLineSpan{{FirstLine: 1, LastLine: 1, Delta: 1}},
		},
		{
			name:    "delete a line",
			oldText: "a\nb\nc\n",
			newText: "a\nc\n",
			want:    []evidence.WriteLineSpan{{FirstLine: 2, LastLine: 2, Delta: -1}},
		},
		{
			name:    "edit inside one line",
			oldText: "abc\ndef\n",
			newText: "aBc\ndef\n",
			want:    []evidence.WriteLineSpan{{FirstLine: 1, LastLine: 1, Delta: 0}},
		},
		{
			name:    "two separate hunks",
			oldText: "a\nb\nc\nd\n",
			newText: "A\nb\nc\nD\n",
			want: []evidence.WriteLineSpan{
				{FirstLine: 1, LastLine: 1, Delta: 0},
				{FirstLine: 4, LastLine: 4, Delta: 0},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := writeLineSpans(tc.oldText, tc.newText)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("writeLineSpans = %+v, want %+v", got, tc.want)
			}
		})
	}
}
