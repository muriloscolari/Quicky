package main

import "testing"

func TestServerURL(t *testing.T) {
	tests := map[string]string{
		"127.0.0.1:8090": "http://127.0.0.1:8090",
		"0.0.0.0:9000":   "http://127.0.0.1:9000",
		":8080":          "http://127.0.0.1:8080",
	}
	for address, want := range tests {
		if got := serverURL(address); got != want {
			t.Errorf("serverURL(%q) = %q, want %q", address, got, want)
		}
	}
}

func TestControlInfoRoundTrip(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	want := controlInfo{PID: 42, Address: "127.0.0.1:8090", Token: "secret"}
	if err := writeControl(want); err != nil {
		t.Fatal(err)
	}
	got, err := readControl()
	if err != nil {
		t.Fatal(err)
	}
	if got.PID != want.PID || got.Address != want.Address || got.Token != want.Token {
		t.Fatalf("readControl() = %+v, want %+v", got, want)
	}
}

func TestParseServeFlags(t *testing.T) {
	addr, open, err := parseServeFlags("daemon", defaultDaemonAddress, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "127.0.0.1:47892" || open {
		t.Fatalf("daemon defaults = (%q, %v), want (127.0.0.1:47892, false)", addr, open)
	}

	addr, open, err = parseServeFlags("serve", defaultAddress, true, []string{"-addr", "127.0.0.1:49000", "-open=false"})
	if err != nil {
		t.Fatal(err)
	}
	if addr != "127.0.0.1:49000" || open {
		t.Fatalf("serve override = (%q, %v), want (127.0.0.1:49000, false)", addr, open)
	}
}
