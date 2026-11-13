package webapp

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hui1601/Quicky/internal/bluez"
	quicky "github.com/hui1601/Quicky/lib"
)

//go:embed web/*
var webFiles embed.FS

type scannedDevice struct {
	Name           string          `json:"name"`
	BLEAddress     string          `json:"bleAddress"`
	ControlAddress string          `json:"controlAddress"`
	OtherAddress   string          `json:"otherAddress,omitempty"`
	RSSI           int16           `json:"rssi"`
	VendorID       uint16          `json:"vendorId"`
	LeftBattery    byte            `json:"leftBattery"`
	RightBattery   byte            `json:"rightBattery"`
	CaseBattery    byte            `json:"caseBattery"`
	LeftCharging   bool            `json:"leftCharging"`
	RightCharging  bool            `json:"rightCharging"`
	CaseCharging   bool            `json:"caseCharging"`
	Product        *quicky.Product `json:"product,omitempty"`
	LastSeen       time.Time       `json:"lastSeen"`
}

type batteryState struct {
	Left          byte `json:"left"`
	Right         byte `json:"right"`
	Case          byte `json:"case"`
	LeftCharging  bool `json:"leftCharging"`
	RightCharging bool `json:"rightCharging"`
	CaseCharging  bool `json:"caseCharging"`
}

type firmwareState struct {
	Left  string `json:"left"`
	Right string `json:"right"`
}

type ancState struct {
	CmdID int `json:"cmdId"`
}

type connectionState struct {
	Connected        bool           `json:"connected"`
	Address          string         `json:"address,omitempty"`
	Device           *scannedDevice `json:"device,omitempty"`
	Battery          *batteryState  `json:"battery,omitempty"`
	Firmware         *firmwareState `json:"firmware,omitempty"`
	ANC              *ancState      `json:"anc,omitempty"`
	AudioAddress     string         `json:"audioAddress,omitempty"`
	AudioReconnected bool           `json:"audioReconnected,omitempty"`
	Warning          string         `json:"warning,omitempty"`
	Since            time.Time      `json:"since,omitempty"`
}

type autoConnectState struct {
	Enabled     bool   `json:"enabled"`
	LastAddress string `json:"lastAddress,omitempty"`
	Paused      bool   `json:"paused,omitempty"`
}

type autoConnectRequest struct {
	Enabled *bool `json:"enabled"`
}

type appState struct {
	mu                sync.RWMutex
	deviceMu          sync.Mutex
	scanner           *quicky.Scanner
	devices           map[string]scannedDevice
	scanning          bool
	connecting        bool
	lastError         string
	client            *quicky.Client
	connection        connectionState
	autoConnect       autoConnectState
	autoConnectPaused bool
	controlToken      string
	shutdown          chan struct{}
	shutdownOnce      sync.Once
}

type statusResponse struct {
	Scanning    bool             `json:"scanning"`
	Connecting  bool             `json:"connecting"`
	LastError   string           `json:"lastError,omitempty"`
	Devices     []scannedDevice  `json:"devices"`
	Connection  connectionState  `json:"connection"`
	AutoConnect autoConnectState `json:"autoConnect"`
}

type connectRequest struct {
	Addresses []string `json:"addresses"`
}

type controlRequest struct {
	Action  string `json:"action"`
	Mode    string `json:"mode,omitempty"`
	CmdID   *int   `json:"cmdId,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
	Left    *int   `json:"left,omitempty"`
	Right   *int   `json:"right,omitempty"`
	Value   *int   `json:"value,omitempty"`
	Name    string `json:"name,omitempty"`
}

// Server owns the HTTP handler and active Bluetooth resources.
type Server struct {
	app     *appState
	handler http.Handler
}

// New creates the local control panel. controlToken protects the shutdown
// endpoint used by the background process manager.
func New(controlToken string) (*Server, error) {
	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		return nil, err
	}
	app := newApp(controlToken)
	return &Server{
		app:     app,
		handler: app.routes(http.FileServer(http.FS(webRoot))),
	}, nil
}

func newApp(controlToken ...string) *appState {
	token := ""
	if len(controlToken) > 0 {
		token = controlToken[0]
	}
	app := &appState{
		devices:      make(map[string]scannedDevice),
		autoConnect:  loadAutoConnect(),
		controlToken: token,
		shutdown:     make(chan struct{}),
	}
	return app
}

// StartAutoConnect starts periodic lightweight BLE discovery to reconnect
// the last paired QCY earphone when it powers on nearby.
func (s *Server) StartAutoConnect() {
	go s.app.autoConnectLoop()
}

// Handler returns the complete API and static frontend handler.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// ShutdownRequested is closed after an authenticated shutdown request.
func (s *Server) ShutdownRequested() <-chan struct{} {
	return s.app.shutdown
}

// Close releases scanning and GATT resources.
func (s *Server) Close() error {
	s.app.stopScan()
	s.app.deviceMu.Lock()
	defer s.app.deviceMu.Unlock()
	return s.app.disconnectCurrent()
}

func (a *appState) routes(static http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", a.handleStatus)
	mux.HandleFunc("POST /api/scan", a.handleScan)
	mux.HandleFunc("POST /api/connect", a.handleConnect)
	mux.HandleFunc("POST /api/disconnect", a.handleDisconnect)
	mux.HandleFunc("POST /api/refresh", a.handleRefresh)
	mux.HandleFunc("POST /api/control", a.handleControl)
	mux.HandleFunc("POST /api/autoconnect", a.handleAutoConnect)
	mux.HandleFunc("POST /api/shutdown", a.handleShutdown)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		static.ServeHTTP(w, r)
	}))
	return mux
}

func (a *appState) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if a.controlToken == "" || subtle.ConstantTimeCompare(
		[]byte(r.Header.Get("X-Quicky-Token")),
		[]byte(a.controlToken),
	) != 1 {
		writeError(w, http.StatusForbidden, "token de controle inválido")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"stopping": true})
	a.shutdownOnce.Do(func() { close(a.shutdown) })
}

func (a *appState) handleStatus(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	devices := make([]scannedDevice, 0, len(a.devices))
	for _, device := range a.devices {
		devices = append(devices, device)
	}
	autoConnect := a.autoConnect
	autoConnect.Paused = a.autoConnectPaused
	response := statusResponse{
		Scanning:    a.scanning,
		Connecting:  a.connecting,
		LastError:   a.lastError,
		Devices:     devices,
		Connection:  a.connection,
		AutoConnect: autoConnect,
	}
	a.mu.RUnlock()

	sort.Slice(response.Devices, func(i, j int) bool {
		return response.Devices[i].RSSI > response.Devices[j].RSSI
	})
	writeJSON(w, http.StatusOK, response)
}

func (a *appState) handleScan(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	if a.scanning {
		a.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]bool{"started": false})
		return
	}
	if a.connecting || a.connection.Connected {
		a.mu.Unlock()
		writeError(w, http.StatusConflict, "desconecte o fone antes de procurar outros dispositivos")
		return
	}

	scanner := quicky.NewScanner()
	a.scanner = scanner
	a.devices = make(map[string]scannedDevice)
	a.scanning = true
	a.lastError = ""
	a.mu.Unlock()

	go a.scan(scanner, 12*time.Second)
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

func (a *appState) scan(scanner *quicky.Scanner, duration time.Duration) {
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(duration):
			_ = scanner.StopScan()
		case <-done:
		}
	}()

	err := scanner.Scan(func(result quicky.ScanResult) {
		if result.Advertisement == nil {
			return
		}

		advertisement := result.Advertisement
		device := scannedDevice{
			Name:           result.Name,
			BLEAddress:     strings.ToUpper(result.Address.String()),
			ControlAddress: strings.ToUpper(advertisement.ControlMAC),
			OtherAddress:   strings.ToUpper(advertisement.OtherMAC),
			RSSI:           result.RSSI,
			VendorID:       advertisement.VendorID,
			LeftBattery:    advertisement.LeftBattery,
			RightBattery:   advertisement.RightBattery,
			CaseBattery:    advertisement.BoxBattery,
			LeftCharging:   advertisement.IsLeftCharging,
			RightCharging:  advertisement.IsRightCharging,
			CaseCharging:   advertisement.IsBoxCharging,
			LastSeen:       time.Now(),
		}
		if product, ok := result.GetProductInfo(); ok {
			device.Product = product
		}

		a.mu.Lock()
		if a.scanner == scanner {
			a.devices[device.BLEAddress] = device
		}
		a.mu.Unlock()
	})
	close(done)

	a.mu.Lock()
	if a.scanner == scanner {
		a.scanning = false
		if err != nil {
			a.lastError = friendlyBluetoothError(err)
		}
	}
	a.mu.Unlock()
}

func (a *appState) handleConnect(w http.ResponseWriter, r *http.Request) {
	var request connectRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	addresses := uniqueAddresses(request.Addresses)
	if len(addresses) == 0 {
		writeError(w, http.StatusBadRequest, "informe pelo menos um endereço Bluetooth válido")
		return
	}

	connection, status, err := a.connectAddresses(r.Context(), addresses)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, connection)
}

func (a *appState) connectAddresses(parentCtx context.Context, addresses []string) (connectionState, int, error) {
	a.stopScan()
	a.mu.Lock()
	if a.connecting {
		a.mu.Unlock()
		return connectionState{}, http.StatusConflict, errors.New("uma conexão já está em andamento")
	}
	a.connecting = true
	a.lastError = ""
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.connecting = false
		a.mu.Unlock()
	}()

	a.deviceMu.Lock()
	defer a.deviceMu.Unlock()
	a.disconnectCurrent()

	var connectionErrors []string
	for _, address := range addresses {
		discoveredDevice := a.findDevice(addresses)
		audioAddress := ""
		audioWasConnected := false
		if discoveredDevice != nil &&
			discoveredDevice.ControlAddress != "" &&
			!sameAddress(discoveredDevice.ControlAddress, address) {
			audioAddress = discoveredDevice.ControlAddress
			audioWasConnected, _ = bluez.DeviceConnected(audioAddress)
		}

		client, err := quicky.New(address)
		if err != nil {
			connectionErrors = append(connectionErrors, fmt.Sprintf("%s: %v", address, err))
			continue
		}

		ctx, cancel := context.WithTimeout(parentCtx, 15*time.Second)
		err = client.Connect(ctx)
		cancel()
		if err != nil {
			connectionErrors = append(connectionErrors, fmt.Sprintf("%s: %v", address, err))
			continue
		}

		connection := connectionState{
			Connected:    true,
			Address:      address,
			Device:       discoveredDevice,
			AudioAddress: audioAddress,
			Since:        time.Now(),
		}
		if audioWasConnected {
			if reconnectErr := bluez.ConnectDevice(audioAddress); reconnectErr != nil {
				connection.Warning = "controle conectado, mas não foi possível restaurar o áudio: " + reconnectErr.Error()
			} else {
				connection.AudioReconnected = true
			}
		}
		if battery, readErr := readBatteryWithRetry(client, 3, 250*time.Millisecond); readErr == nil {
			connection.Battery = batteryResponse(battery)
		} else if discoveredDevice != nil {
			connection.Battery = batteryFromAdvertisement(discoveredDevice)
		}
		if version, readErr := readVersionWithRetry(client, 2, 200*time.Millisecond); readErr == nil {
			connection.Firmware = &firmwareState{Left: version.Left, Right: version.Right}
		}

		a.mu.Lock()
		a.client = client
		a.connection = connection
		a.lastError = ""
		a.autoConnectPaused = false
		a.autoConnect.LastAddress = address
		cfg := a.autoConnect
		a.mu.Unlock()
		saveAutoConnect(cfg)
		go a.listenEvents(client)
		return connection, http.StatusOK, nil
	}

	err := errors.New(strings.Join(connectionErrors, "; "))
	message := friendlyBluetoothError(err)
	a.mu.Lock()
	a.lastError = message
	a.mu.Unlock()
	return connectionState{}, http.StatusBadGateway, errors.New(message)
}

func (a *appState) handleDisconnect(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	a.autoConnectPaused = true
	a.mu.Unlock()

	a.deviceMu.Lock()
	err := a.disconnectCurrent()
	a.deviceMu.Unlock()
	if err != nil {
		writeError(w, http.StatusBadGateway, friendlyBluetoothError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"disconnected": true})
}

func (a *appState) handleAutoConnect(w http.ResponseWriter, r *http.Request) {
	var request autoConnectRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.Enabled == nil {
		writeError(w, http.StatusBadRequest, "campo enabled é obrigatório")
		return
	}

	a.mu.Lock()
	a.autoConnect.Enabled = *request.Enabled
	if *request.Enabled {
		a.autoConnectPaused = false
	}
	state := a.autoConnect
	state.Paused = a.autoConnectPaused
	a.mu.Unlock()
	saveAutoConnect(state)
	writeJSON(w, http.StatusOK, state)
}

func (a *appState) handleRefresh(w http.ResponseWriter, _ *http.Request) {
	a.deviceMu.Lock()
	defer a.deviceMu.Unlock()
	client, err := a.connectedClient()
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	battery, err := client.ReadBattery()
	if err != nil {
		writeError(w, http.StatusBadGateway, friendlyBluetoothError(err))
		return
	}
	batteryState := batteryResponse(battery)
	version, versionErr := client.ReadVersion()

	a.mu.Lock()
	a.connection.Battery = batteryState
	if versionErr == nil {
		a.connection.Firmware = &firmwareState{Left: version.Left, Right: version.Right}
	}
	connection := a.connection
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, connection)
}

func (a *appState) handleControl(w http.ResponseWriter, r *http.Request) {
	var request controlRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	a.deviceMu.Lock()
	defer a.deviceMu.Unlock()
	client, err := a.connectedClient()
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	switch request.Action {
	case "anc":
		a.mu.RLock()
		var prod *quicky.Product
		if a.connection.Device != nil {
			prod = a.connection.Device.Product
		}
		a.mu.RUnlock()

		var cmdID int
		cmdID, err = sendANCCommand(client, prod, request)
		if err == nil {
			a.mu.Lock()
			if a.client == client {
				a.connection.ANC = &ancState{CmdID: cmdID}
			}
			a.mu.Unlock()
		}
	case "low_latency":
		err = withBool(request.Enabled, client.SetLowLatency)
	case "in_ear":
		err = withBool(request.Enabled, client.SetInEarDetection)
	case "spatial_audio":
		err = withBool(request.Enabled, client.SetSpatialAudio)
	case "ldac":
		err = withBool(request.Enabled, client.SetLDAC)
	case "sleep":
		err = withBool(request.Enabled, client.SetSleepMode)
	case "volume":
		if err = validateRange(request.Left, "volume esquerdo", 0, 100); err == nil {
			if err = validateRange(request.Right, "volume direito", 0, 100); err == nil {
				err = client.SetVolume(byte(*request.Left), byte(*request.Right))
			}
		}
	case "balance":
		if err = validateRange(request.Value, "balanço", 0, 100); err == nil {
			err = client.SetSoundBalance(byte(*request.Value))
		}
	case "tone_volume":
		if err = validateRange(request.Value, "volume dos avisos", 0, 100); err == nil {
			err = client.SetToneVolume(byte(*request.Value))
		}
	case "rename":
		name := strings.TrimSpace(request.Name)
		if name == "" || len([]byte(name)) > 32 {
			err = errors.New("o nome deve ter entre 1 e 32 bytes")
		} else {
			err = client.SetName(name)
		}
	default:
		err = errors.New("controle desconhecido")
	}

	if err != nil {
		writeError(w, http.StatusBadRequest, friendlyBluetoothError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}

func (a *appState) stopScan() {
	a.mu.RLock()
	scanner := a.scanner
	scanning := a.scanning
	a.mu.RUnlock()
	if scanning && scanner != nil {
		_ = scanner.StopScan()
	}
}

func (a *appState) disconnectCurrent() error {
	a.mu.Lock()
	client := a.client
	a.client = nil
	a.connection = connectionState{}
	a.mu.Unlock()
	if client != nil {
		return client.Disconnect()
	}
	return nil
}

func (a *appState) connectedClient() (*quicky.Client, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connection.Connected
	a.mu.RUnlock()
	if client == nil || !connected || !client.Connected() {
		return nil, errors.New("nenhum fone está conectado")
	}
	return client, nil
}

func (a *appState) findDevice(addresses []string) *scannedDevice {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, device := range a.devices {
		for _, address := range addresses {
			if sameAddress(device.BLEAddress, address) || sameAddress(device.ControlAddress, address) {
				copy := device
				return &copy
			}
		}
	}
	return nil
}

func batteryResponse(battery quicky.Battery) *batteryState {
	return &batteryState{
		Left:          battery.Left.Level,
		Right:         battery.Right.Level,
		Case:          battery.Case.Level,
		LeftCharging:  battery.Left.Charging,
		RightCharging: battery.Right.Charging,
		CaseCharging:  battery.Case.Charging,
	}
}

func batteryFromAdvertisement(device *scannedDevice) *batteryState {
	if device == nil {
		return nil
	}
	return &batteryState{
		Left:          device.LeftBattery,
		Right:         device.RightBattery,
		Case:          device.CaseBattery,
		LeftCharging:  device.LeftCharging,
		RightCharging: device.RightCharging,
		CaseCharging:  device.CaseCharging,
	}
}

func readBatteryWithRetry(client *quicky.Client, attempts int, delay time.Duration) (quicky.Battery, error) {
	var err error
	var battery quicky.Battery
	for i := 0; i < attempts; i++ {
		battery, err = client.ReadBattery()
		if err == nil {
			return battery, nil
		}
		if i+1 < attempts {
			time.Sleep(delay)
		}
	}
	return battery, err
}

func readVersionWithRetry(client *quicky.Client, attempts int, delay time.Duration) (quicky.Version, error) {
	var err error
	var version quicky.Version
	for i := 0; i < attempts; i++ {
		version, err = client.ReadVersion()
		if err == nil {
			return version, nil
		}
		if i+1 < attempts {
			time.Sleep(delay)
		}
	}
	return version, err
}

func (a *appState) listenEvents(client *quicky.Client) {
	for ev := range client.Events() {
		a.mu.Lock()
		if a.client != client {
			a.mu.Unlock()
			return
		}
		switch ev.Type {
		case quicky.EventBattery:
			if battery, ok := ev.Parsed.(quicky.Battery); ok {
				a.connection.Battery = batteryResponse(battery)
			}
		case quicky.EventANCSetting:
			if anc, ok := ev.Parsed.(quicky.ANCSetting); ok {
				cmdID := (int(anc.Mode) << 16) | (int(anc.SubScene) << 8) | int(anc.NoiseValue)
				a.connection.ANC = &ancState{CmdID: cmdID}
			}
		case quicky.EventNoiseCancelMode:
			if mode, ok := ev.Parsed.(byte); ok {
				a.connection.ANC = &ancState{CmdID: int(mode)}
			}
		}
		a.mu.Unlock()
	}
}

func resolveANCCmdID(prod *quicky.Product, request controlRequest) (int, error) {
	if request.CmdID != nil {
		if *request.CmdID < 0 || *request.CmdID > 0xFFFFFF {
			return 0, errors.New("cmdId ANC inválido")
		}
		return *request.CmdID, nil
	}

	if prod != nil && prod.Features.ANC != nil {
		for _, mode := range prod.Features.ANC.Modes {
			if matchesANCModeName(mode.Name, request.Mode) {
				if mode.DefaultCmd > 0 {
					return mode.DefaultCmd, nil
				}
				return mode.StartCmdID, nil
			}
			for _, item := range mode.Items {
				if matchesANCModeName(item.Name, request.Mode) {
					return item.StartCmdID, nil
				}
			}
		}
	}

	legacyModes := map[string]int{
		"off":          int(quicky.NoiseCancelOff),
		"anc":          int(quicky.NoiseCancelANC),
		"outdoor":      int(quicky.NoiseCancelOutdoor),
		"transparency": int(quicky.NoiseCancelTransparency),
	}
	cmdID, ok := legacyModes[request.Mode]
	if !ok {
		return 0, errors.New("modo ANC inválido")
	}
	return cmdID, nil
}

func matchesANCModeName(productName, modeKey string) bool {
	lower := strings.ToLower(strings.TrimSpace(productName))
	switch modeKey {
	case "off":
		return lower == "normal" || lower == "off"
	case "anc":
		return strings.Contains(lower, "noise cancel") || lower == "anc"
	case "transparency":
		return strings.Contains(lower, "transparency")
	case "outdoor":
		return strings.Contains(lower, "outdoor") || strings.Contains(lower, "wind")
	default:
		return lower == strings.ToLower(strings.TrimSpace(modeKey))
	}
}

func sendANCCommand(client *quicky.Client, prod *quicky.Product, request controlRequest) (int, error) {
	cmdID, err := resolveANCCmdID(prod, request)
	if err != nil {
		return 0, err
	}
	if cmdID <= 0xFF {
		return cmdID, client.SetNoiseCancelMode(quicky.NoiseCancelMode(cmdID))
	}
	mode := byte((cmdID >> 16) & 0xFF)
	subScene := byte((cmdID >> 8) & 0xFF)
	noiseValue := byte(cmdID & 0xFF)
	return cmdID, client.SetANCSetting(mode, subScene, noiseValue)
}

func uniqueAddresses(addresses []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		address = strings.ToUpper(strings.TrimSpace(address))
		if address == "" || address == "00:00:00:00:00:00" || seen[address] {
			continue
		}
		seen[address] = true
		result = append(result, address)
	}
	return result
}

func sameAddress(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func withBool(value *bool, send func(bool) error) error {
	if value == nil {
		return errors.New("estado ligado/desligado não informado")
	}
	return send(*value)
}

func validateRange(value *int, name string, min, max int) error {
	if value == nil {
		return fmt.Errorf("%s não informado", name)
	}
	if *value < min || *value > max {
		return fmt.Errorf("%s deve estar entre %d e %d", name, min, max)
	}
	return nil
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("JSON inválido: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON inválido: apenas um objeto é permitido")
	}
	return nil
}

func friendlyBluetoothError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "qcy service not found"):
		return message + ". O endereço não expõe o serviço QCY; tente o MAC BLE ou desligue o Bluetooth do celular"
	case strings.Contains(lower, "not all characteristics found"):
		return message + ". Este firmware pode usar um protocolo QCY diferente do padrão"
	case strings.Contains(lower, "operation already in progress"):
		return message + ". Feche outros aplicativos Bluetooth e tente novamente"
	case strings.Contains(lower, "no default controller"), strings.Contains(lower, "adapter"):
		return message + ". Confirme que o BlueZ está ativo e o Bluetooth está desbloqueado"
	default:
		return message
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("erro ao escrever resposta: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (a *appState) autoConnectLoop() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.shutdown:
			return
		case <-ticker.C:
			a.tryAutoConnect()
		}
	}
}

func (a *appState) tryAutoConnect() {
	a.mu.Lock()
	if !a.autoConnect.Enabled || a.autoConnectPaused || a.autoConnect.LastAddress == "" ||
		a.scanning || a.connecting || a.connection.Connected {
		a.mu.Unlock()
		return
	}
	target := a.autoConnect.LastAddress
	scanner := quicky.NewScanner()
	a.scanner = scanner
	a.scanning = true
	a.mu.Unlock()

	a.scan(scanner, 5*time.Second)

	if dev := a.findDevice([]string{target}); dev != nil {
		_, _, _ = a.connectAddresses(context.Background(), []string{target})
	}
}

func autoConnectPath() (string, error) {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "quicky", "autoconnect.json"), nil
}

func loadAutoConnect() autoConnectState {
	state := autoConnectState{Enabled: true}
	path, err := autoConnectPath()
	if err != nil {
		return state
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	_ = json.Unmarshal(data, &state)
	return state
}

func saveAutoConnect(state autoConnectState) {
	path, err := autoConnectPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	state.Paused = false
	data, err := json.Marshal(state)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}
