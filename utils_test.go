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
		// GitHub Container Registry
		{"github package published", `{"action":"published","package":{"owner":{"login":"myorg"},"package_type":"CONTAINER","name":"myapp"}}`, "myorg/myapp"},
		{"github package no owner", `{"package":{"package_type":"CONTAINER","name":"myapp"}}`, ""},
		{"github package non-container type", `{"package":{"package_type":"NPM","name":"myapp"}}`, ""},
		{"github package empty name", `{"package":{"owner":{"login":"myorg"},"package_type":"CONTAINER","name":""}}`, ""},
		// Generic
		{"generic image", `{"image":"myregistry/myapp"}`, "myregistry/myapp"},
		{"generic empty image", `{"image":"","tag":"latest"}`, ""},
		// Docker registry v2
		{"docker registry push", `{"events":[{"action":"push","target":{"repository":"myorg/myapp","mediaType":"application/vnd.docker.distribution.manifest.v2+json"}}]}`, "myorg/myapp"},
		{"docker registry push no media type", `{"events":[{"action":"push","target":{"repository":"myorg/myapp"}}]}`, "myorg/myapp"},
		{"docker registry blob push", `{"events":[{"action":"push","target":{"repository":"myorg/myapp","mediaType":"application/octet-stream"}}]}`, "myorg/myapp"},
		{"docker registry multiple events", `{"events":[{"action":"push","target":{"repository":"myorg/app1","mediaType":"application/vnd.docker.distribution.manifest.v2+json"}},{"action":"push","target":{"repository":"myorg/app2","mediaType":"application/vnd.docker.distribution.manifest.v2+json"}}]}`, "myorg/app1"},
		{"docker registry empty events", `{"events":[]}`, ""},
		{"docker registry no repository", `{"events":[{"action":"push","target":{}}]}`, ""},
		{"docker registry pull", `{"events":[{"action":"pull","target":{"repository":"myorg/myapp"}}]}`, "myorg/myapp"},
		{"docker registry delete", `{"events":[{"action":"delete","target":{"repository":"myorg/myapp"}}]}`, "myorg/myapp"},
		{"docker registry mixed actions", `{"events":[{"action":"pull","target":{"repository":"myorg/myapp"}},{"action":"push","target":{"repository":"myorg/myapp","mediaType":"application/vnd.docker.distribution.manifest.v2+json"}}]}`, "myorg/myapp"},
		{"docker registry oci index", `{"events":[{"action":"push","target":{"repository":"myorg/myapp","mediaType":"application/vnd.oci.image.index.v1+json"}}]}`, "myorg/myapp"},
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

func TestIsRelevantEvent(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected bool
	}{
		{"push manifest", `{"events":[{"action":"push","target":{"repository":"myorg/myapp","mediaType":"application/vnd.docker.distribution.manifest.v2+json"}}]}`, true},
		{"push no media type", `{"events":[{"action":"push","target":{"repository":"myorg/myapp"}}]}`, true},
		{"push blob", `{"events":[{"action":"push","target":{"repository":"myorg/myapp","mediaType":"application/octet-stream"}}]}`, false},
		{"pull", `{"events":[{"action":"pull","target":{"repository":"myorg/myapp"}}]}`, false},
		{"delete", `{"events":[{"action":"delete","target":{"repository":"myorg/myapp"}}]}`, false},
		{"oci index", `{"events":[{"action":"push","target":{"repository":"myorg/myapp","mediaType":"application/vnd.oci.image.index.v1+json"}}]}`, true},
		{"oci manifest", `{"events":[{"action":"push","target":{"repository":"myorg/myapp","mediaType":"application/vnd.oci.image.manifest.v1+json"}}]}`, true},
		{"generic image", `{"image":"myapp"}`, true},
		{"forgejo package", `{"package":{"owner":{"login":"myorg"},"type":"container","name":"myapp"}}`, true},
		{"empty events", `{"events":[]}`, false},
		{"empty body", `{}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRelevantEvent([]byte(tt.body))
			if result != tt.expected {
				t.Errorf("isRelevantEvent() = %v, want %v", result, tt.expected)
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
		{
			name:    "valid X-Hub-Signature-256",
			headers: map[string]string{"X-Hub-Signature-256": validSig},
			secrets: []string{secret},
			want:    true,
		},
		{
			name:    "valid X-Hub-Signature",
			headers: map[string]string{"X-Hub-Signature": validSig},
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

func TestExtractPayload(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		expected    string
	}{
		{"json content type", `{"image":"myapp"}`, "application/json", `{"image":"myapp"}`},
		{"form encoded with payload", "payload=%7B%22image%22%3A%22myapp%22%7D", "application/x-www-form-urlencoded", `{"image":"myapp"}`},
		{"form encoded no payload", "other=data", "application/x-www-form-urlencoded", "other=data"},
		{"empty content type", `{"image":"myapp"}`, "", `{"image":"myapp"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(extractPayload([]byte(tt.body), tt.contentType))
			if result != tt.expected {
				t.Errorf("extractPayload() = %q, want %q", result, tt.expected)
			}
		})
	}
}
