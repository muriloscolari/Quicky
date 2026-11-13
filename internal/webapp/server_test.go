package webapp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hui1601/Quicky/internal/product"
	quicky "github.com/hui1601/Quicky/lib"
)

func TestStatusWithoutBluetooth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	response := httptest.NewRecorder()
	newApp().routes(http.NotFoundHandler()).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}

	var status statusResponse
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Connection.Connected || status.Scanning || status.Connecting {
		t.Fatalf("unexpected active status: %+v", status)
	}
}

func TestUniqueAddresses(t *testing.T) {
	got := uniqueAddresses([]string{
		"aa:bb:cc:dd:ee:ff",
		"AA:BB:CC:DD:EE:FF",
		"00:00:00:00:00:00",
		" 11:22:33:44:55:66 ",
		"",
	})
	want := []string{"AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueAddresses() = %v, want %v", got, want)
	}
}

func TestShutdownRequiresControlToken(t *testing.T) {
	server, err := New("secret")
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	unauthorizedResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusForbidden {
		t.Fatalf("unauthorized status = %d, want %d", unauthorizedResponse.Code, http.StatusForbidden)
	}

	authorized := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	authorized.Header.Set("X-Quicky-Token", "secret")
	authorizedResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(authorizedResponse, authorized)
	if authorizedResponse.Code != http.StatusAccepted {
		t.Fatalf("authorized status = %d, want %d", authorizedResponse.Code, http.StatusAccepted)
	}

	select {
	case <-server.ShutdownRequested():
	default:
		t.Fatal("shutdown channel was not closed")
	}
}

func TestResolveANCCmdIDForMeloBudsProAndLegacy(t *testing.T) {
	meloBudsPro, ok := product.Lookup(19786)
	if !ok {
		t.Fatal("expected QCY MeloBuds Pro (19786) in product database")
	}

	cases := []struct {
		name string
		prod *quicky.Product
		req  controlRequest
		want int
	}{
		{name: "melobuds anc", prod: meloBudsPro, req: controlRequest{Mode: "anc"}, want: 65794},
		{name: "melobuds transparency", prod: meloBudsPro, req: controlRequest{Mode: "transparency"}, want: 196868},
		{name: "melobuds off", prod: meloBudsPro, req: controlRequest{Mode: "off"}, want: 131072},
		{name: "legacy off", prod: nil, req: controlRequest{Mode: "off"}, want: int(quicky.NoiseCancelOff)},
		{name: "legacy anc", prod: nil, req: controlRequest{Mode: "anc"}, want: int(quicky.NoiseCancelANC)},
	}

	for _, tc := range cases {
		got, err := resolveANCCmdID(tc.prod, tc.req)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}

	explicit := 66816
	got, err := resolveANCCmdID(meloBudsPro, controlRequest{CmdID: &explicit})
	if err != nil || got != explicit {
		t.Fatalf("explicit cmdId: got (%d, %v), want (%d, nil)", got, err, explicit)
	}
}

func TestBatteryFromAdvertisement(t *testing.T) {
	got := batteryFromAdvertisement(&scannedDevice{
		LeftBattery:  82,
		RightBattery: 81,
		CaseBattery:  0,
		LeftCharging: true,
	})
	if got == nil || got.Left != 82 || got.Right != 81 || got.Case != 0 || !got.LeftCharging {
		t.Fatalf("unexpected battery fallback: %+v", got)
	}
}

func TestAutoConnectEndpointAndPersistence(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	app := newApp()
	handler := app.routes(http.NotFoundHandler())

	body := bytes.NewBufferString(`{"enabled":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/autoconnect", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got autoConnectState
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatalf("expected autoConnect.Enabled = false")
	}

	reloaded := loadAutoConnect()
	if reloaded.Enabled {
		t.Fatalf("expected persisted autoConnect.Enabled = false")
	}
}
