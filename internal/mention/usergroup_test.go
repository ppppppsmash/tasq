package mention

import (
	"testing"
)

func TestParseUserGroups(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "single group",
			text: "<!subteam^S111>",
			want: []string{"S111"},
		},
		{
			name: "group with label",
			text: "<!subteam^S111|@sales>",
			want: []string{"S111"},
		},
		{
			name: "multiple groups",
			text: "<!subteam^S111> <!subteam^S222>",
			want: []string{"S111", "S222"},
		},
		{
			name: "duplicate groups",
			text: "<!subteam^S111> <!subteam^S111>",
			want: []string{"S111"},
		},
		{
			name: "no groups",
			text: "hello <@U111>",
			want: nil,
		},
		{
			name: "mixed with user mentions",
			text: "<@U111> <!subteam^S222> <@U333>",
			want: []string{"S222"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseUserGroups(tt.text)
			if !equal(got, tt.want) {
				t.Errorf("ParseUserGroups(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
