package main

import (
	"context"
	"time"

	"github.com/edaniels/golog"
	bm "github.com/maxhorowitz/btprov/ble/manager"
	bp "github.com/maxhorowitz/btprov/ble/peripheral"
	wf "github.com/maxhorowitz/btprov/wifi"
)

func main() {
	ctx := context.Background()

	// Spin up a BLE connection, wait for required credentials, and cleanly shut down when finished.
	bLogger := golog.NewDebugLogger("BLE manager")
	bluetoothManager, err := bm.NewBluetoothWiFiProvisioner(ctx, bLogger, "Max Horowitz Raspberry Pi 5")
	if err != nil {
		bLogger.Fatalw("failed to initialize bluetooth manager", "err", err)
	}
	if err := bluetoothManager.AcceptIncomingConnections(ctx); err != nil {
		bLogger.Fatalw("failed to accept incoming connections", "err", err)
	}

	// Show example call to "UpdateAvailableWiFiNetworks" which should update the read-only list of available
	// networks advertised by the bluetooth service.
	networks := &bp.AvailableWiFiNetworks{
		Networks: []*struct {
			Ssid        string  "json:\"ssid\""
			Strength    float64 "json:\"strength\""
			RequiresPsk bool    "json:\"requires_psk\""
		}{
			{
				Ssid:        "Viam",
				Strength:    0.75,
				RequiresPsk: true,
			},
			{
				Ssid:        "Viam-2G",
				Strength:    0.3,
				RequiresPsk: true,
			},
		},
	}

	// Show second example call to "UpdateAvailableWiFiNetworks" (read-only values will be distinct from above).
	bluetoothManager.UpdateAvailableWiFiNetworks(ctx, networks)
	bLogger.Info("updated WiFi networks (first)")
	time.Sleep(time.Second * 45)
	networks = &bp.AvailableWiFiNetworks{
		Networks: []*struct {
			Ssid        string  "json:\"ssid\""
			Strength    float64 "json:\"strength\""
			RequiresPsk bool    "json:\"requires_psk\""
		}{
			{
				Ssid:        "Max-Replaced-The-WiFi",
				Strength:    0.75,
				RequiresPsk: true,
			},
			{
				Ssid:        "Viam-5G",
				Strength:    0.3,
				RequiresPsk: true,
			},
		},
	}
	bluetoothManager.UpdateAvailableWiFiNetworks(ctx, networks)
	bLogger.Info("updated WiFi networks (second)")

	credentials, err := bluetoothManager.WaitForCredentials(ctx)
	if err != nil {
		bLogger.Fatalw("failed to wait for credentials", "err", err)
	}
	if err := bluetoothManager.RejectIncomingConnections(ctx); err != nil {
		bLogger.Fatalw("failed to reject incoming connections", "err", err)
	}

	// Once Wi-Fi credentials are transmitted over bluetooth, prepare
	// network manager for Wi-Fi connection.
	wLogger := golog.NewDebugLogger("Wi-Fi manager")
	lwf, err := wf.NewLinuxWiFiManager(ctx, wLogger)
	if err != nil {
		wLogger.Fatalf("failed to set up Wi-Fi manager: %v", err)
	}
	if err := lwf.ConnectToWiFi(ctx, credentials.GetSSID(), credentials.GetPsk()); err != nil {
		wLogger.Fatalf("failed to connect to Wi-Fi: %v", err)
	}
}
