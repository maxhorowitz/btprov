package provisioningmanager

import (
	"context"
	"fmt"
	"time"

	nm "github.com/Wifx/gonetworkmanager"
	"github.com/edaniels/golog"
	"github.com/godbus/dbus/v5"
	"github.com/pkg/errors"
	"go.viam.com/utils"
)

type ProvisioningManager interface {
	ConnectToWiFi(ctx context.Context, logger golog.Logger, ssid, psk string) error
}

type linuxProvisioningManager struct {
	networkManager nm.NetworkManager
	device         nm.DeviceWireless
}

func NewProvisioningManager(ctx context.Context, logger golog.Logger) (*linuxProvisioningManager, error) {
	networkManager, err := nm.NewNetworkManager()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to connect to network manager")
	}
	if err := networkManager.SetPropertyWirelessEnabled(true); err != nil {
		return nil, errors.WithMessage(err, "failed to set property wireless enabled")
	}
	count := 0
	for {
		if !utils.SelectContextOrWait(ctx, time.Second) {
			return nil, ctx.Err()
		}
		count++
		enabled, err := networkManager.GetPropertyWirelessEnabled()
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get property wireless enabled")
		}
		if enabled {
			break
		}
		if count == 10 {
			return nil, errors.New("failed to verify whether wireless is enabled")
		}
	}

	// Get a list of network devices
	devices, err := networkManager.GetDevices()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get devices")
	}

	// Find a Wi-Fi device
	var wifiDevice nm.DeviceWireless = nil
	for _, device := range devices {
		deviceType, err := device.GetPropertyDeviceType()
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get device type")
		}

		//nolint:exhaustive
		switch deviceType {
		case nm.NmDeviceTypeEthernet:
			ethernetDev, ok := device.(nm.DeviceWired)
			if !ok {
				return nil, errors.WithMessage(err, "failed to cast \"ethernet\" device type to \"wired\" device type")
			}
			ifName, err := ethernetDev.GetPropertyInterface()
			if err != nil {
				return nil, errors.WithMessage(err, "failed to get property interface")
			}
			logger.Infof("recognized ethernet interface: %s, but looking for Wi-Fi", ifName)
			continue
		case nm.NmDeviceTypeWifi:
			wifiDev, ok := device.(nm.DeviceWireless)
			if !ok {
				return nil, errors.WithMessage(err, "failed to cast \"Wi-Fi\" device type to \"wireless\" device type")
			}
			ifName, err := wifiDev.GetPropertyInterface()
			if err != nil {
				return nil, errors.WithMessage(err, "failed to get property interface")
			}
			logger.Infof("recognized Wi-Fi interface: %s", ifName)
			wifiDevice = wifiDev
		default:
			continue
		}
		if wifiDevice != nil {
			break
		}
	}
	if wifiDevice == nil {
		return nil, errors.New("no Wi-Fi device found")
	}

	return &linuxProvisioningManager{
		networkManager: networkManager,
		device:         wifiDevice,
	}, nil
}

func (lpm *linuxProvisioningManager) ConnectToWiFi(ctx context.Context, logger golog.Logger, ssid, psk string) error {
	wifiDevice := lpm.device
	if err := wifiDevice.SetPropertyManaged(true); err != nil {
		return errors.WithMessage(err, "failed to set Wi-Fi to \"managed\"")
	}

	// Record original system D-Bus NetworkManager properties (before scanning and attempting to connect to new Wi-Fi).
	originalScan, err := wifiDevice.GetPropertyLastScan()
	if err != nil {
		return errors.WithMessage(err, "failure getting original scan of system D-Bus NetworkManager properties")
	}

	// Connect to D-Bus system bus
	conn, err := dbus.SystemBus()
	if err != nil {
		return errors.WithMessage(err, "failed to connect to system D-Bus, cannot listen for changes to Wi-Fi properties (NetworkManager)")
	}

	// Scan for available Wi-Fi networks
	if err := wifiDevice.RequestScan(); err != nil {
		return errors.WithMessage(err, "failed to scan for Wi-Fi networks")
	}

	// Add a match rule for NetworkManager properties changes
	matchRule := fmt.Sprintf(
		"type='signal'," +
			"interface='org.freedesktop.DBus.Properties'," +
			"member='PropertiesChanged'," +
			"arg0='org.freedesktop.NetworkManager.Device.Wireless'",
	)
	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		return errors.WithMessage(err, "failed to add match rule for system D-Bus NetworkManager properties changes")
	}
	// See if can replace the above 'conn.BusObject().Call()' with `conn.AddMatchSignal()`
	// if err := conn.AddMatchSignal(dbus.WithMatchArg(0, matchRule)); err != nil {
	// 	return errors.WithMessage(err, "failed to add match rule for NetworkManager properties changes")
	// }
	signals := make(chan *dbus.Signal) // Unbuffered channel to ensure blocking writes (no dropped signals).
	conn.Signal(signals)

	// Listen for D-Bus messages which signify a "new" last scan.
	recordedNewScan := false
	for {
		if err := ctx.Err(); err != nil {
			return errors.WithMessage(err, "failure getting changes to Wi-Fi properties")
		}
		select {
		case <-ctx.Done():
			return errors.WithMessage(ctx.Err(), "failure getting changes to Wi-Fi properties")
		case signal := <-signals:
			// Expecting: org.freedesktop.DBus.Properties::PropertiesChanged
			if len(signal.Body) < 2 {
				continue
			}

			// Extract the changed properties
			changedProps, ok := signal.Body[1].(map[string]dbus.Variant)
			if !ok {
				continue
			}

			// Check if "LastScan" property has changed
			lastScan, exists := changedProps["LastScan"]
			if exists {
				logger.Infof(
					"recorded change to \"LastScan\" value (%v --> %v) in D-Bus NetworkManager properties, "+
						"we are now ready to get Wi-Fi access points", originalScan, lastScan.Value(),
				)
				recordedNewScan = true
				break
			}
			logger.Info("recorded unrelated change to D-Bus properties")
		default:
		}
		if recordedNewScan {
			break
		}
		time.Sleep(time.Second)
	}

	// Wait for scan results
	logger.Infof("attempting to get available wifi networks...")
	accessPoints, err := wifiDevice.GetAllAccessPoints()
	if err != nil {
		return errors.WithMessage(err, "unable to get all access points for Wi-Fi device")
	}

	var requestedAccessPoint nm.AccessPoint = nil
	for _, ap := range accessPoints {
		apSSID, err := ap.GetPropertySSID()
		if err != nil {
			return errors.WithMessage(err, "unable to get access point SSID")
		}
		apStrength, err := ap.GetPropertyStrength()
		if err != nil {
			return errors.WithMessage(err, "unable to get access point strength")
		}
		logger.Infof(" - SSID: %s, Strength: %d", apSSID, apStrength)
		if requestedAccessPoint == nil && apSSID == ssid {
			requestedAccessPoint = ap
		}
	}
	if requestedAccessPoint == nil {
		return errors.Errorf("failed to discover access point with SSID: %s", ssid)
	}

	// Create a new Wi-Fi connection profile
	connection := nm.ConnectionSettings{
		"connection": map[string]interface{}{
			"type": "802-11-wireless",
		},
		"802-11-wireless": map[string]interface{}{
			"ssid": ssid,
			"mode": "infrastructure",
		},
		"802-11-wireless-security": map[string]interface{}{
			"key-mgmt": "wpa-psk",
			"psk":      psk,
		},
		"ipv4": map[string]interface{}{
			"method": "auto",
		},
		"ipv6": map[string]interface{}{
			"method": "ignore",
		},
	}

	// Attempt to make the Wi-Fi connection.
	if _, err := lpm.networkManager.AddAndActivateWirelessConnection(connection, lpm.device, requestedAccessPoint); err != nil {
		return errors.WithMessagef(err, "failed to connect to Wi-Fi")
	}
	logger.Infof("successfully connected to Wi-Fi: %s", ssid)
	return nil
}
