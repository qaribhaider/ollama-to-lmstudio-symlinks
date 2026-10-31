package main

import (
	"reflect"
	"testing"

	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/models"
)

func TestFilterByNames_KeepsSelectionOrderAndIgnoresUnknown(t *testing.T) {
	items := []string{"a", "b", "c"}
	got := filterByNames(items, []string{"c", "missing", "a"}, func(s string) string { return s })
	if want := []string{"c", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("filterByNames = %v, want %v", got, want)
	}
}

func TestCountLinked(t *testing.T) {
	created, skipped := countLinked([]int{1, 2, 3, 4, 5}, func(n int) bool { return n%2 == 1 })
	if created != 3 || skipped != 2 {
		t.Errorf("countLinked = (%d, %d), want (3, 2)", created, skipped)
	}
}

func TestModelAnnotation(t *testing.T) {
	tests := []struct {
		name  string
		model models.LMStudioModel
		want  string
	}{
		{"plain model", models.LMStudioModel{}, ""},
		{"vision model", models.LMStudioModel{ProjectorPath: "/m/mmproj-x.gguf"}, " [vision: mmproj-x.gguf]"},
		{"sharded model", models.LMStudioModel{ShardPaths: []string{"b", "c"}}, " [3 shards]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modelAnnotation(tt.model); got != tt.want {
				t.Errorf("modelAnnotation = %q, want %q", got, tt.want)
			}
		})
	}
}
