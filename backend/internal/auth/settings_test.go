package auth

import "testing"

func TestSettingsRejectUnsafeDestinations(t *testing.T) {
	valid := Settings{Issuer: "https://identity.host.test", ClientID: "management", Origin: "https://manage.host.test", Mode: "production"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Settings){
		func(s *Settings) { s.Origin = "http://manage.host.test" },
		func(s *Settings) { s.Issuer = "http://identity.host.test" },
		func(s *Settings) { s.Origin = "https://manage.host.test/path" },
		func(s *Settings) { s.Origin = "https://manage.host.test/?next=https://attacker.test" },
		func(s *Settings) { s.Origin = "https://user:pass@manage.host.test" },
		func(s *Settings) { s.ClientID = "" },
		func(s *Settings) { s.Mode = "local" },
	} {
		s := valid
		change(&s)
		if s.Validate() == nil {
			t.Fatal("accepted unsafe authentication settings")
		}
	}
	local := Settings{Issuer: "http://localhost:5556", ClientID: "pds-local", Origin: "http://localhost:5173", Mode: "local"}
	if err := local.Validate(); err != nil {
		t.Fatal(err)
	}
	local.Origin = "http://0.0.0.0:5173"
	if local.Validate() == nil {
		t.Fatal("local mode accepted non-loopback origin")
	}
}
