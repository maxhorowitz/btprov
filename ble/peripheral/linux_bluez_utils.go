package bleperipheral

import (
	"fmt"
	"strings"

	"github.com/edaniels/golog"
	"github.com/godbus/dbus"
	"github.com/pkg/errors"
)

const (
	BluezDBusService  = "org.bluez"
	BluezAgentPath    = "/custom/agent"
	BluezAgentManager = "org.bluez.AgentManager1"
	BluezAgent        = "org.bluez.Agent1"
)

// registerAgent registers a custom pairing agent to handle pairing requests.
func registerAgent(logger golog.Logger) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("failed to connect to system DBus: %w", err)
	}

	// Export agent methods
	reply := conn.Export(nil, BluezAgentPath, BluezAgent)
	if reply != nil {
		return fmt.Errorf("failed to export agent: %w", reply)
	}

	// Register the agent
	obj := conn.Object(BluezDBusService, "/org/bluez")
	call := obj.Call("org.bluez.AgentManager1.RegisterAgent", 0, dbus.ObjectPath(BluezAgentPath), "NoInputNoOutput")
	if call.Err != nil {
		return fmt.Errorf("failed to register agent: %w", call.Err)
	}

	// Set as the default agent
	call = obj.Call("org.bluez.AgentManager1.RequestDefaultAgent", 0, dbus.ObjectPath(BluezAgentPath))
	if call.Err != nil {
		return fmt.Errorf("failed to set default agent: %w", call.Err)
	}

	logger.Info("bluetooth pairing agent registered.")
	return nil
}

// listenForPairing waits for an incoming BLE pairing request and automatically trusts the device.
func listenForPairing(logger golog.Logger) error {
	if err := registerAgent(logger); err != nil {
		return errors.WithMessage(err, "failed to register BlueZ agent")
	}

	conn, err := dbus.SystemBus()
	if err != nil {
		return errors.WithMessage(err, "failed to connect to system DBus")
	}

	// Listen for properties changed events
	signalChan := make(chan *dbus.Signal, 10)
	conn.Signal(signalChan)

	// Add a match rule to listen for DBus property changes
	matchRule := "type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged'"
	err = conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err
	if err != nil {
		return errors.WithMessage(err, "Failed to add DBus match rule")
	}

	logger.Info("waiting for a BLE pairing request...")

	for signal := range signalChan {
		// Check if the signal is from a BlueZ device
		if len(signal.Body) < 3 {
			continue
		}

		iface, ok := signal.Body[0].(string)
		if !ok || iface != "org.bluez.Device1" {
			continue
		}

		// Check if the "Paired" property is in the event
		changedProps, ok := signal.Body[1].(map[string]dbus.Variant)
		if !ok {
			continue
		}

		paired, exists := changedProps["Connected"]
		if !exists || paired.Value() != true {
			continue
		}

		// Extract device path from the signal sender
		devicePath := string(signal.Path)

		// Convert DBus object path to MAC address
		deviceMAC := convertDBusPathToMAC(devicePath)
		if deviceMAC == "" {
			continue
		}

		logger.Infof("device %s initiated pairing!", deviceMAC)

		// Mark device as trusted
		if err = trustDevice(logger, devicePath); err != nil {
			return errors.WithMessage(err, "failed to trust device")
		} else {
			logger.Info("device successfully trusted!")
		}
	}
	return nil
}

// trustDevice sets the device as trusted and connects to it
func trustDevice(logger golog.Logger, devicePath string) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("failed to connect to DBus: %w", err)
	}

	obj := conn.Object(BluezDBusService, dbus.ObjectPath(devicePath))

	// Set Trusted = true
	call := obj.Call("org.freedesktop.DBus.Properties.Set", 0,
		"org.bluez.Device1", "Trusted", dbus.MakeVariant(true))
	if call.Err != nil {
		return fmt.Errorf("failed to set Trusted property: %w", call.Err)
	}
	logger.Info("device marked as trusted.")

	return nil
}

// convertDBusPathToMAC converts a DBus object path to a Bluetooth MAC address
func convertDBusPathToMAC(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		return ""
	}

	// Extract last part and convert underscores to colons
	macPart := parts[len(parts)-1]
	mac := strings.ReplaceAll(macPart, "_", ":")
	return mac
}
