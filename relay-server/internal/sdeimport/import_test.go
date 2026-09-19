package sdeimport

import (
	"strings"
	"testing"
)

func TestLocalizedBestPrefersChineseAndFallsBackToEnglish(t *testing.T) {
	if got := (localized{"zh": " 吉他 ", "en": "Jita"}).Best(); got != "吉他" {
		t.Fatalf("got %q", got)
	}
	if got := (localized{"zh": " ", "en": " Jita "}).Best(); got != "Jita" {
		t.Fatalf("got %q", got)
	}
}
func TestDecodeJSONLFields(t *testing.T) {
	rows, err := decodeJSONL(strings.NewReader(`{"_key":34,"name":{"zh":"氚","en":"Tritanium"},"published":true,"packagedVolume":0.01,"volume":0.01}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Key != 34 || rows[0].Name.Best() != "氚" || rows[0].PackagedVolume == nil || *rows[0].PackagedVolume != .01 {
		t.Fatalf("bad rows: %+v", rows)
	}
}
func TestDecodeJSONLRejectsMalformedInput(t *testing.T) {
	if _, err := decodeJSONL(strings.NewReader("{\n")); err == nil {
		t.Fatal("expected error")
	}
}
