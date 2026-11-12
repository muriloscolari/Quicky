# QuiCkY

[English](README.md) | [한국어](README.ko.md)

Go library for controlling QCY Bluetooth earphones on Linux via BLE GATT.

> **Proof of concept.** No guarantee it will work properly. Use at your own risk.

## Install

```bash
go get github.com/hui1601/Quicky
```

Requires BlueZ on Linux.

## Web Control Panel

This repository includes a local web interface for discovering and controlling
QCY earphones. It runs entirely on your computer and only listens on
`127.0.0.1` by default.

Requirements:

- Go 1.23.1 or newer
- BlueZ running on Linux
- A Bluetooth adapter with BLE support

### Start and stop

The recommended workflow builds the binary and keeps the server running in the
background:

```bash
make start
make status
make restart
make stop
```

Follow the server log with:

```bash
make logs
```

The background process uses a private control token under `$XDG_RUNTIME_DIR`
and writes logs to `$XDG_STATE_HOME/quicky/server.log` (normally
`~/.local/state/quicky/server.log`). `stop` requests a graceful shutdown, which
disconnects GATT and stops any active BLE scan before the process exits.

The same operations are available directly from the compiled binary:

```bash
make build
./bin/quicky-control start
./bin/quicky-control status
./bin/quicky-control logs -f
./bin/quicky-control stop
```

### Foreground mode

For development or troubleshooting, run in the current terminal:

```bash
go run ./cli
```

Use `Ctrl+C` to stop it. The control panel opens at
<http://127.0.0.1:8090>.

Available commands and options:

```bash
./bin/quicky-control help
./bin/quicky-control start -open=false
./bin/quicky-control restart -addr 127.0.0.1:9090
./bin/quicky-control serve -open=false
```

Recommended connection flow:

1. Pair the earphones normally in the Linux Bluetooth settings for audio.
2. Temporarily disable Bluetooth on phones that know the earphones.
3. Take both earbuds out of the case.
4. Start the control panel and select **Procurar QCY**.
5. Connect the detected device. The app uses the address that emitted the BLE
   advertisement, keeping the Classic Bluetooth/A2DP address untouched.

The UI controls the QCY BLE service, not the Linux audio profile. PipeWire or
PulseAudio remains responsible for A2DP, microphone selection, and codec
negotiation. A2DP audio and the BLE control connection should work at the same
time. The advertised `ControlMAC` may be the Classic audio address on some QCY
models, so the UI displays it for diagnostics but never tries it automatically.
Some firmware, including the QCY MeloBuds Pro, briefly drops A2DP when GATT is
opened. If audio was connected before the operation, the server asks BlueZ to
restore that Classic connection after GATT is ready. Features vary by model and
firmware.

If no device appears, verify the Linux Bluetooth service:

```bash
systemctl status bluetooth
bluetoothctl show
rfkill list bluetooth
```

## Usage

### Scanning for Devices

```go
package main

import (
	"fmt"

	quicky "github.com/hui1601/Quicky/lib"
)

func main() {
	scanner := quicky.NewScanner()
	scanner.Scan(func(result quicky.ScanResult) {
		fmt.Printf("Found: %s (RSSI: %d)\n", result.Address.String(), result.RSSI)
		if result.Advertisement != nil {
			fmt.Printf("  Battery: L=%d%% R=%d%% Box=%d%%\n",
				result.Advertisement.LeftBattery,
				result.Advertisement.RightBattery,
				result.Advertisement.BoxBattery)
			fmt.Printf("  Control MAC: %s\n", result.Advertisement.ControlMAC)
			
			if product, ok := result.GetProductInfo(); ok {
				fmt.Printf("  Model: %s\n", product.Title)
				if product.Features.ANC != nil {
					fmt.Printf("    ANC: %d modes\n", len(product.Features.ANC.Modes))
				}
			}
		}
	})
}
```

### Connecting and Controlling

```go
package main

import (
	"context"
	"fmt"
	"time"

	quicky "github.com/hui1601/Quicky/lib"
)

func main() {
	client, _ := quicky.New("AA:BB:CC:DD:EE:FF")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client.Connect(ctx)
	defer client.Disconnect()

	battery, _ := client.ReadBattery()
	fmt.Printf("Battery: L=%d%% R=%d%% Box=%d%%\n",
		battery.Left.Level, battery.Right.Level, battery.Case.Level)

	client.SetNoiseCancelMode(quicky.NoiseCancelANC)
	client.SetLowLatency(true)
	client.SetVolume(80, 80)
}
```

### Receiving Events

```go
go func() {
	for ev := range client.Events() {
		fmt.Printf("Event: type=%d cmdID=0x%02x\n", ev.Type, ev.CmdID)
	}
}()
```

## Features

- **Discovery** — Scan for QCY devices via BLE manufacturer data (CompanyID `0x521c`), parse battery levels, charging state, and MAC addresses from advertisements
- **Model identification** — Embedded product database (199 models) maps vendorId to model name and supported features (ANC, EQ, etc.)
- **40+ commands** — Noise cancellation, EQ (v1/v2/per-channel), key function mapping, LED effects, spatial audio, wearing detection, alarms, music control, and more
- **Event stream** — Async notification handling with typed event parsing
- **Direct reads** — Battery and firmware version via characteristic reads

## Documentation

- [Protocol Reference](docs/protocol.md)
- [GATT Service UUIDs](docs/service.md)
