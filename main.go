package main

import (
	"context"

	"github.com/edaniels/golog"
	bm "github.com/maxhorowitz/btprov/ble/manager"
	wf "github.com/maxhorowitz/btprov/wifi"
)

func main() {
	ctx := context.Background()

	// Spin up a BLE connection, wait for required credentials, and cleanly shut down when finished.
	bLogger := golog.NewDebugLogger("BLE manager")
	bluetoothManager, err := bm.NewBluetoothManager(ctx, bLogger, "Max Horowitz Raspberry Pi 5")
	if err != nil {
		bLogger.Fatalw("failed to initialize bluetooth manager", "err", err)
	}
	if err := bluetoothManager.AcceptIncomingConnections(ctx); err != nil {
		bLogger.Fatalw("failed to accept incoming connections", "err", err)
	}
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
