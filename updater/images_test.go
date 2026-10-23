package updater

import (
	"testing"
)

func TestNormalizeImage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"myimage", "myimage"},
		{"myimage:latest", "myimage"},
		{"myimage@sha256:abc123", "myimage"},
		{"myimage:v1@sha256:abc123", "myimage"},
		{"registry.example.com/myimage", "registry.example.com/myimage"},
		{"registry.example.com/myimage:latest", "registry.example.com/myimage"},
		{"registry.example.com/myimage:v1@sha256:deadbeef", "registry.example.com/myimage"},
		{"registry.example.com/myimage@sha256:deadbeef", "registry.example.com/myimage"},
		{"reg:8080/myimage", "reg:8080/myimage"},
		{"reg:8080/myimage:latest", "reg:8080/myimage"},
		{"reg:8080/myimage:v1", "reg:8080/myimage"},
		{"reg:8080/myimage:v1@sha256:abc123", "reg:8080/myimage"},
		{"reg:8080/my/org/myimage:v2", "reg:8080/my/org/myimage"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeImage(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeImage(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractImageTag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"myimage", "latest"},
		{"myimage:latest", "latest"},
		{"myimage:v1", "v1"},
		{"myimage:dev", "dev"},
		{"myimage@sha256:abc123", "latest"},
		{"myimage:v1@sha256:abc123", "v1"},
		{"registry.example.com/myimage", "latest"},
		{"registry.example.com/myimage:latest", "latest"},
		{"registry.example.com/myimage:v2", "v2"},
		{"registry.example.com/myimage:v1@sha256:deadbeef", "v1"},
		{"reg:8080/myimage", "latest"},
		{"reg:8080/myimage:latest", "latest"},
		{"reg:8080/myimage:v1", "v1"},
		{"reg:8080/myimage:v1@sha256:abc123", "v1"},
		{"reg:8080/my/org/myimage:v2", "v2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractImageTag(tt.input)
			if result != tt.expected {
				t.Errorf("extractImageTag(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeTag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "latest"},
		{"latest", "latest"},
		{"v1", "v1"},
		{"dev", "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeTag(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeTag(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
