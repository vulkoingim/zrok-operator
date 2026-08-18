package build

import (
	"runtime/debug"
	"testing"
)

func TestRevisionFromSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{name: "empty", want: "none"},
		{
			name:     "full sha",
			settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "26e4b87abcdef0123456789"}},
			want:     "26e4b87",
		},
		{
			name: "dirty",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "26e4b87abcdef0123456789"},
				{Key: "vcs.modified", Value: "true"},
			},
			want: "26e4b87-dirty",
		},
		{
			name: "clean short",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc"},
				{Key: "vcs.modified", Value: "false"},
			},
			want: "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := revisionFromSettings(tt.settings); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
