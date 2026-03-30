package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
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

func TestExtractImage(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		// Forgejo
		{"forgejo with owner", `{"package":{"owner":{"login":"myorg"},"type":"container","name":"myapp"}}`, "myorg/myapp"},
		{"forgejo no owner", `{"package":{"type":"container","name":"myapp"}}`, ""},
		{"forgejo non-container type", `{"package":{"type":"npm","name":"myapp"}}`, ""},
		{"forgejo empty name", `{"package":{"type":"container","name":""}}`, ""},
		// Generic
		{"generic image", `{"image":"myregistry/myapp"}`, "myregistry/myapp"},
		{"generic empty image", `{"image":"","tag":"latest"}`, ""},
		// Edge cases
		{"malformed", `{"repository":`, ""},
		{"garbage", `not json at all`, ""},
		{"empty body", `{}`, ""},
		{"empty bytes", ``, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractImage([]byte(tt.body))
			if result != tt.expected {
				t.Errorf("extractImage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func makeSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestValidateSignature(t *testing.T) {
	body := []byte(`{"test": "data"}`)
	secret := "mysecret"
	validSig := makeSignature(secret, body)

	tests := []struct {
		name    string
		headers map[string]string
		secrets []string
		want    bool
	}{
		{
			name:    "valid X-Forgejo-Signature",
			headers: map[string]string{"X-Forgejo-Signature": validSig},
			secrets: []string{secret},
			want:    true,
		},
		{
			name:    "valid X-Gitea-Signature",
			headers: map[string]string{"X-Gitea-Signature": validSig},
			secrets: []string{secret},
			want:    true,
		},
		{
			name:    "no signature header",
			headers: map[string]string{},
			secrets: []string{secret},
			want:    false,
		},
		{
			name:    "wrong secret",
			headers: map[string]string{"X-Forgejo-Signature": validSig},
			secrets: []string{"wrongsecret"},
			want:    false,
		},
		{
			name:    "multiple secrets one valid",
			headers: map[string]string{"X-Forgejo-Signature": validSig},
			secrets: []string{"wrong1", secret, "wrong2"},
			want:    true,
		},
		{
			name:    "signature without sha256 prefix",
			headers: map[string]string{"X-Forgejo-Signature": strings.TrimPrefix(validSig, "sha256=")},
			secrets: []string{secret},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			for k, v := range tt.headers {
				headers.Set(k, v)
			}

			result := validateSignature(tt.secrets, headers, body)
			if result != tt.want {
				t.Errorf("validateSignature() = %v, want %v", result, tt.want)
			}
		})
	}
}
