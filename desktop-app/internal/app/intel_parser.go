package app

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"eve-assistant/desktop-app/internal/protocol"
)

var intelSystems = []string{"Jita", "Amarr", "Dodixie", "Rens", "Hek", "Tama", "Niarja", "Uedama", "Perigen Falls", "Aldranette"}
var intelShips = []string{"Rifter", "Merlin", "Drake", "Caracal", "Raven", "Megathron", "Catalyst", "Venture", "Interceptor"}

// ParseIntel intentionally only emits entities from the supplied text; it never fetches or invents EVE data.
func ParseIntel(text, source string, now time.Time) []protocol.IntelFinding {
	text = strings.TrimSpace(text)
	if text == "" {
		return []protocol.IntelFinding{}
	}
	if source == "" {
		source = "clipboard"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	out := make([]protocol.IntelFinding, 0)
	add := func(kind, value string, confidence float64) {
		out = append(out, protocol.IntelFinding{Kind: kind, Value: value, Stance: "unknown", Source: source, Summary: fmt.Sprintf("%s detected", value), ObservedAt: now, ExpiresAt: now.Add(30 * time.Minute), Confidence: confidence})
	}
	for _, value := range intelSystems {
		if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(value) + `\b`).MatchString(text) {
			add("system", value, .95)
		}
	}
	for _, value := range intelShips {
		if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(value) + `\b`).MatchString(text) {
			add("ship", value, .88)
		}
	}
	lower := strings.ToLower(text)
	stance := ""
	for _, term := range []string{"敌对", "红名", "hostile", "war target", "neut"} {
		if strings.Contains(lower, strings.ToLower(term)) {
			stance = "hostile"
			break
		}
	}
	if stance == "" {
		for _, term := range []string{"友军", "蓝名", "friendly", "allied", "蓝"} {
			if strings.Contains(lower, strings.ToLower(term)) {
				stance = "friendly"
				break
			}
		}
	}
	if stance != "" {
		out = append(out, protocol.IntelFinding{Kind: "stance", Value: stance, Stance: stance, Source: source, Summary: fmt.Sprintf("%s detected (%s)", stance, stance), ObservedAt: now, ExpiresAt: now.Add(30 * time.Minute), Confidence: .82})
	}
	return out
}
