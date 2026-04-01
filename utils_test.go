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

func TestGenerateEndpointPath(t *testing.T) {
	t.Run("produces GUID format", func(t *testing.T) {
		path := generateEndpointPath("secret", 0)
		if len(path) != 36 {
			t.Errorf("expected 36-char GUID, got %d chars: %q", len(path), path)
		}
		parts := strings.Split(path, "-")
		if len(parts) != 5 {
			t.Errorf("expected 5 hyphen-separated parts, got %d: %q", len(parts), path)
		}
		lengths := []int{8, 4, 4, 4, 12}
		for i, part := range parts {
			if len(part) != lengths[i] {
				t.Errorf("part %d: expected %d chars, got %d: %q", i, lengths[i], len(part), part)
			}
		}
	})

	t.Run("is deterministic", func(t *testing.T) {
		a := generateEndpointPath("mysecret", 0)
		b := generateEndpointPath("mysecret", 0)
		if a != b {
			t.Errorf("same inputs produced different outputs: %q vs %q", a, b)
		}
	})

	t.Run("different secrets produce different paths", func(t *testing.T) {
		a := generateEndpointPath("secret1", 0)
		b := generateEndpointPath("secret2", 0)
		if a == b {
			t.Errorf("different secrets produced same path: %q", a)
		}
	})

	t.Run("different indices produce different paths", func(t *testing.T) {
		a := generateEndpointPath("secret", 0)
		b := generateEndpointPath("secret", 1)
		if a == b {
			t.Errorf("different indices produced same path: %q", a)
		}
	})

	t.Run("matches expected HMAC-SHA256 output", func(t *testing.T) {
		secret := "testsecret"
		index := 5
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte("5"))
		fullHash := hex.EncodeToString(mac.Sum(nil))
		expected := fullHash[0:8] + "-" + fullHash[8:12] + "-" + fullHash[12:16] + "-" + fullHash[16:20] + "-" + fullHash[20:32]

		result := generateEndpointPath(secret, index)
		if result != expected {
			t.Errorf("generateEndpointPath() = %q, want %q", result, expected)
		}
	})
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
			name:    "valid bearer token",
			headers: map[string]string{"Authorization": "Bearer " + secret},
			secrets: []string{secret},
			want:    true,
		},
		{
			name:    "invalid bearer token",
			headers: map[string]string{"Authorization": "Bearer wrongtoken"},
			secrets: []string{secret},
			want:    false,
		},
		{
			name:    "bearer with multiple secrets one valid",
			headers: map[string]string{"Authorization": "Bearer " + secret},
			secrets: []string{"wrong1", secret, "wrong2"},
			want:    true,
		},
		{
			name:    "bearer with wrong auth scheme",
			headers: map[string]string{"Authorization": "Basic dXNlcjpwYXNz"},
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
