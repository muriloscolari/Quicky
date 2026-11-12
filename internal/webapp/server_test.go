package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
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
