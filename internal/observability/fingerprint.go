package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

type Fingerprint string

var contextLabels = map[string]struct{}{
	"severity":    {},
	"summary":     {},
	"description": {},
}

func DeriveFingerprint(alert Alert) Fingerprint {
	parts := []string{"name=" + alert.Name}
	keys := make([]string, 0, len(alert.Labels))
	for key := range alert.Labels {
		if _, ok := contextLabels[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key+"="+alert.Labels[key])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return Fingerprint(hex.EncodeToString(sum[:]))
}
