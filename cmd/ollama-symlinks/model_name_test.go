package main

import "testing"

func TestIsSafeOllamaModelName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"lms-llama3:latest", true},
		{"hf.co/user/model-q4:Q4_K_M", true},
		{"-x:tag", false}, // would be read as a flag by `ollama rm`
		{"model", false},  // no tag
		{"a/../b:tag", false},
		{"bad name:tag", false},
		{"model:tag;rm", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafeOllamaModelName(tt.name); got != tt.want {
				t.Errorf("isSafeOllamaModelName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
