package mention

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "simple mentions",
			text: "<@U111> <@U222> please check",
			want: []string{"U111", "U222"},
		},
		{
			name: "duplicate mentions",
			text: "<@U111> <@U111> <@U222>",
			want: []string{"U111", "U222"},
		},
		{
			name: "exclude half-width parens",
			text: "<@U111> (<@U222>) <@U333>",
			want: []string{"U111", "U333"},
		},
		{
			name: "exclude full-width parens",
			text: "<@U111> （<@U222>） <@U333>",
			want: []string{"U111", "U333"},
		},
		{
			name: "exclude after CC:",
			text: "<@U111> <@U222> CC: <@U333> <@U444>",
			want: []string{"U111", "U222"},
		},
		{
			name: "exclude after cc:",
			text: "<@U111> cc: <@U222>",
			want: []string{"U111"},
		},
		{
			name: "exclude after full-width CC：",
			text: "<@U111> CC：<@U222>",
			want: []string{"U111"},
		},
		{
			name: "no mentions",
			text: "hello world",
			want: nil,
		},
		{
			name: "mention with display name",
			text: "<@U111|tanaka> <@U222|suzuki>",
			want: []string{"U111", "U222"},
		},
		{
			name: "CC on second line only",
			text: "<@U111> <@U222>\ncc: <@U333>",
			want: []string{"U111", "U222"},
		},
		{
			name: "parens and CC combined",
			text: "<@U111> (<@U222>) some text CC: <@U333>",
			want: []string{"U111"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.text)
			if !equal(got, tt.want) {
				t.Errorf("Parse(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func equal(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
