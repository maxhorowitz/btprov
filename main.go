package main

import (
	"context"

	"github.com/edaniels/golog"
	bm "github.com/maxhorowitz/btprov/ble/manager"
	bp "github.com/maxhorowitz/btprov/ble/peripheral"
	wf "github.com/maxhorowitz/btprov/wifi"
)

func main() {
	ctx := context.Background()

	// Spin up a BLE connection, wait for required credentials, and cleanly shut down when finished.
	bLogger := golog.NewDebugLogger("BLE manager")
	blep, err := bp.NewLinuxBLEPeripheral(ctx, bLogger, "Max Horowitz Raspberry Pi5")
	if err != nil {
		bLogger.Fatalf("failed to set up BLE manager: %v", err)
	}
	if err := blep.StartAdvertising(); err != nil {
		bLogger.Fatalf("failed to start advertising characteristics in BLE: %v", err)
	}
	credentials, err := bm.WaitForCredentials(ctx, bLogger, blep)
	if err != nil {
		bLogger.Fatalf("failed to get all required values over BLE: %v", err)
	}
	if err := blep.StopAdvertising(); err != nil {
		bLogger.Fatalf("failed to stop advertising characteristics in BLE: %v", err)
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
