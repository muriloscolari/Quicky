package bluez

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const deviceInterface = "org.bluez.Device1"

// DeviceConnected reports the BlueZ connection state for an address.
func DeviceConnected(address string) (bool, error) {
	connection, path, err := findDevice(address)
	if err != nil {
		return false, err
	}
	property, err := connection.Object("org.bluez", path).GetProperty(deviceInterface + ".Connected")
	if err != nil {
		return false, err
	}
	connected, ok := property.Value().(bool)
	if !ok {
		return false, errors.New("BlueZ returned an invalid Connected property")
	}
	return connected, nil
}

// ConnectDevice restores all configured profiles for an existing BlueZ device.
func ConnectDevice(address string) error {
	connection, path, err := findDevice(address)
	if err != nil {
		return err
	}
	device := connection.Object("org.bluez", path)
	property, propertyErr := device.GetProperty(deviceInterface + ".Connected")
	if propertyErr == nil {
		if connected, ok := property.Value().(bool); ok && connected {
			return nil
		}
	}
	if err := device.Call(deviceInterface+".Connect", 0).Err; err != nil {
		return fmt.Errorf("connect %s: %w", address, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		property, propertyErr := device.GetProperty(deviceInterface + ".Connected")
		if propertyErr == nil {
			if connected, ok := property.Value().(bool); ok && connected {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s to reconnect", address)
}

func findDevice(address string) (*dbus.Conn, dbus.ObjectPath, error) {
	connection, err := dbus.SystemBus()
	if err != nil {
		return nil, "", err
	}

	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	err = connection.Object("org.bluez", "/").Call(
		"org.freedesktop.DBus.ObjectManager.GetManagedObjects",
		0,
	).Store(&objects)
	if err != nil {
		return nil, "", err
	}

	for path, interfaces := range objects {
		properties, ok := interfaces[deviceInterface]
		if !ok {
			continue
		}
		addressProperty, exists := properties["Address"]
		if !exists {
			continue
		}
		deviceAddress, ok := addressProperty.Value().(string)
		if ok && strings.EqualFold(deviceAddress, address) {
			return connection, path, nil
		}
	}
	return nil, "", fmt.Errorf("BlueZ device %s not found", address)
}
