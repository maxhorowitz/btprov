package provisioningmanager

import (
	"context"
	"fmt"
	"log"
	"time"

	nm "github.com/Wifx/gonetworkmanager"
	"github.com/edaniels/golog"
	"github.com/pkg/errors"
	"go.viam.com/utils"
)

type ProvisioningManager interface {
	ConnectToWiFi(ssid, psk string) error
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
	for {
		if !utils.SelectContextOrWait(ctx, time.Second) {
			return nil, ctx.Err()
		}
		enabled, err := networkManager.GetPropertyWirelessEnabled()
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get property wireless enabled")
		}
		if enabled {
			break
		}
	}

	// Get a list of network devices
	devices, err := networkManager.GetDevices()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get devices")
	}

	// Find a Wi-Fi device
	var wifiDevice nm.DeviceWireless
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
			logger.Infof("recognized ethernet interface: %s, but looking for wifi", ifName)
			continue
		case nm.NmDeviceTypeWifi:
			wifiDev, ok := device.(nm.DeviceWireless)
			if !ok {
				return nil, errors.WithMessage(err, "failed to cast \"wifi\" device type to \"wireless\" device type")
			}
			ifName, err := wifiDev.GetPropertyInterface()
			if err != nil {
				return nil, errors.WithMessage(err, "failed to get property interface")
			}
			logger.Infof("recognized wifi interface: %s", ifName)
			wifiDevice = wifiDev
			break
		default:
			continue
		}
	}
	if wifiDevice == nil {
		return nil, errors.New("no wifi device found")
	}

	return &linuxProvisioningManager{
		networkManager: networkManager,
		device:         wifiDevice,
	}, nil
}

func (lpm *linuxProvisioningManager) ConnectToWiFi(ssid, psk string) error {
	wifiDevice := lpm.device

	wifiDevice.SetPropertyManaged(true)

	// Scan for available Wi-Fi networks
	err := wifiDevice.RequestScan()
	if err != nil {
		return errors.WithMessage(err, "Failed to scan Wi-Fi networks")
	}

	// Wait for scan results
	fmt.Println("Attempting to get available WiFi networks...")
	accessPoints, _ := wifiDevice.GetAllAccessPoints()
	attemptCounter := 0
	for {
		if attemptCounter == 5 {
			log.Fatal("Unable to get access points, exiting program")
		}
		time.Sleep(time.Second)

		// Get available access points
		accessPoints, err := wifiDevice.GetAllAccessPoints()
		if err != nil {
			log.Default().Printf("Failed to get access points: %v, will retry", err)
			continue
		}

		// Display available Wi-Fi networks
		if len(accessPoints) == 0 {
			log.Default().Printf("No access points found, will retry")
			continue
		}
		fmt.Print("\nFound nearby access points!")
		for _, ap := range accessPoints {
			ssid, _ := ap.GetPropertySSID()
			strength, _ := ap.GetPropertyStrength()
			fmt.Printf("SSID: %s, Strength: %d\n", ssid, strength)
		}
		break
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
	for _, ap := range accessPoints {
		ssid, _ := ap.GetPropertySSID()
		fmt.Printf("- %s", ssid)
		if ssid == "Viam" {
			ac, err := lpm.networkManager.AddAndActivateWirelessConnection(connection, lpm.device, ap)
			if err != nil {
				return errors.WithMessage(err, "Failed to connect to WiFi")
			}
			fmt.Printf("\nSuccessully connected to WiFi: %s\n", ac)
		}
	}
	return nil
}
