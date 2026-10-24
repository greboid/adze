package updater

import (
	"strings"
)

func normalizeImage(ref string) string {
	if strings.Contains(ref, "@") {
		ref = strings.SplitN(ref, "@", 2)[0]
	}
	if idx := strings.LastIndex(ref, ":"); idx > strings.LastIndex(ref, "/") {
		ref = ref[:idx]
	}
	return ref
}

func extractImageTag(ref string) string {
	if strings.Contains(ref, "@") {
		ref = strings.SplitN(ref, "@", 2)[0]
	}
	if idx := strings.LastIndex(ref, ":"); idx > strings.LastIndex(ref, "/") {
		return ref[idx+1:]
	}
	return "latest"
}

func normalizeTag(tag string) string {
	if tag == "" {
		return "latest"
	}
	return tag
}
