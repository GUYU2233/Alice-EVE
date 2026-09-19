package esisync

import "testing"

func TestExtendedDomainsAreReadOnlyPersonalScopes(t *testing.T) {
	want := map[string]string{
		"clones": "esi-clones.read_clones.v1", "implants": "esi-clones.read_implants.v1",
		"transactions": "esi-wallet.read_character_wallet.v1", "order_history": "esi-markets.read_character_orders.v1",
		"contracts": "esi-contracts.read_character_contracts.v1", "mail_headers": "esi-mail.read_mail.v1",
		"fittings": "esi-fittings.read_fittings.v1", "industry_jobs": "esi-industry.read_character_jobs.v1",
	}
	seen := map[string]bool{}
	for _, kind := range Kinds {
		seen[kind] = true
	}
	for kind, scope := range want {
		if !seen[kind] {
			t.Errorf("missing kind %s", kind)
		}
		if RequiredScopes[kind] != scope {
			t.Errorf("%s scope=%q", kind, RequiredScopes[kind])
		}
	}
	for kind, scope := range RequiredScopes {
		if scope == "" {
			continue
		}
		if contains(scope, ".write_") || contains(scope, ".manage_") || contains(scope, "corporation_") || contains(scope, "corporations.") {
			t.Errorf("unsafe/non-personal scope %s=%s", kind, scope)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
