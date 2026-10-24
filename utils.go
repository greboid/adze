package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func validateSignature(secrets []string, headers http.Header, body []byte) bool {
	sig := headers.Get("X-Forgejo-Signature")
	if sig == "" {
		sig = headers.Get("X-Gitea-Signature")
	}
	if sig == "" {
		sig = headers.Get("X-Hub-Signature-256")
	}
	if sig == "" {
		sig = headers.Get("X-Hub-Signature")
	}
	if sig == "" {
		return validateBearer(secrets, headers)
	}

	sig = strings.TrimPrefix(sig, "sha256=")

	for _, secret := range secrets {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return true
		}
	}

	return false
}

func validateBearer(secrets []string, headers http.Header) bool {
	auth := headers.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	for _, secret := range secrets {
		if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1 {
			return true
		}
	}
	return false
}

func extractPayload(body []byte, contentType string) []byte {
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		if values, err := url.ParseQuery(string(body)); err == nil {
			if payload := values.Get("payload"); payload != "" {
				return []byte(payload)
			}
		}
	}
	return body
}

func extractImage(body []byte) string {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return ""
	}
	if p.Package.Owner.Login != "" && p.Package.Name != "" &&
		(p.Package.Type == "container" || p.Package.PackageType == "CONTAINER") {
		image := p.Package.Owner.Login + "/" + p.Package.Name
		if u, err := url.Parse(p.Package.Owner.HTMLURL); err == nil && u.Host != "" {
			image = u.Host + "/" + image
		}
		return image
	}
	if p.Image != "" {
		return p.Image
	}
	for _, event := range p.Events {
		if event.Target.Repository != "" {
			if event.Request.Host != "" {
				return event.Request.Host + "/" + event.Target.Repository
			}
			return event.Target.Repository
		}
	}
	return ""
}

func isRelevantEvent(body []byte) bool {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return false
	}
	if p.Package.Owner.Login != "" || p.Image != "" {
		return true
	}
	for _, event := range p.Events {
		if event.Action != "push" {
			continue
		}
		if event.Target.MediaType != "" && !strings.Contains(event.Target.MediaType, "manifest") && !strings.Contains(event.Target.MediaType, "image.index") {
			continue
		}
		if event.Target.Repository != "" {
			return true
		}
	}
	return false
}

func generateEndpointPath(secret string, index int) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.Itoa(index)))
	b := mac.Sum(nil)[:16]
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func extractTag(body []byte) string {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return ""
	}
	if p.Package.Owner.Login != "" && p.Package.Name != "" &&
		(p.Package.Type == "container" || p.Package.PackageType == "CONTAINER") {
		if p.Package.Version != "" {
			return p.Package.Version
		}
		return p.Package.PackageVersion.ContainerMetadata.Tag.Name
	}
	for _, event := range p.Events {
		if event.Target.Tag != "" {
			return event.Target.Tag
		}
	}
	return ""
}
