package app

import "testing"

func TestParseGameLogCombat(t *testing.T) {
	tests := []struct{ line, action string }{
		{`[ 2026.09.16 04:52:30 ] (combat) <color=0xffcc0000><b>125</b></color> from <b>Hostile Pilot</b> - Hits`, "under_attack"},
		{`[ 2026.09.16 04:52:31 ] (combat) <b>Hostile Pilot</b> misses you completely`, "under_attack"},
		{`[ 2026.09.16 04:52:32 ] (combat) <b>88</b> to <b>Hostile Pilot</b> - Weapon - Hits`, "attacking"},
		{`[ 2026.09.16 04:52:33 ] (combat) Warp scramble attempt from Hostile Pilot to you!`, "warp_scrambled"},
	}
	for _, tt := range tests {
		got, ok := parseGameLogLine(tt.line)
		if !ok || got.Action != tt.action || got.Target == "" {
			t.Fatalf("line=%q got=%+v ok=%v", tt.line, got, ok)
		}
	}
}
func TestParseGameLogIgnoresNoise(t *testing.T) {
	if _, ok := parseGameLogLine(`[ 2026.09.16 04:52:34 ] (info) ordinary message`); ok {
		t.Fatal("noise emitted")
	}
}
