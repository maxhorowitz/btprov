package main

import (
	"context"
	"sync"
	"time"

	"github.com/edaniels/golog"
	bm "github.com/maxhorowitz/btprov/ble_manager"
	wf "github.com/maxhorowitz/btprov/wifi_manager"
	"go.viam.com/utils"
)

func main() {
	// bm.Old()
	new()
}

func new() {
	ctx := context.Background()

	// Spin up a BLE connection for accepting Wi-Fi credentials.
	bLogger := golog.NewDebugLogger("BLE manager")
	blem, err := bm.NewLinuxBLEPeripheral(ctx, bLogger, "Max Horowitz Raspberry Pi5")
	if err != nil {
		bLogger.Fatalf("failed to set up BLE manager: %v", err)
	}
	if err := blem.StartAdvertising(); err != nil {
		bLogger.Fatalf("failed to start advertising characteristics in BLE: %v", err)
	}

	var ssid, psk, robotPartKeyID, robotPartKey string
	wg := &sync.WaitGroup{}

	// Read SSID
	wg.Add(1)
	utils.ManagedGo(func() {
		for {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
			}
			v, err := blem.ReadSsid()
			if err != nil {
				bLogger.Errorf("failed to read ssid: %s", v)
				return
			}
			if v != "" {
				ssid = v
				return
			}
		}
	}, wg.Done)

	// Read Passkey
	wg.Add(1)
	utils.ManagedGo(func() {
		for {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
			}
			v, err := blem.ReadPsk()
			if err != nil {
				bLogger.Errorf("failed to read psk: %s", v)
				return
			}
			if v != "" {
				psk = v
				return
			}
		}
	}, wg.Done)

	// Read Robot Part Key ID
	wg.Add(1)
	utils.ManagedGo(func() {
		for {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
			}
			v, err := blem.ReadRobotPartKeyID()
			if err != nil {
				bLogger.Errorf("failed to read robot part key ID: %s", v)
				return
			}
			if v != "" {
				robotPartKeyID = v
				return
			}
		}
	}, wg.Done)

	// Read Robot Part Key
	wg.Add(1)
	utils.ManagedGo(func() {
		for {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
			}
			v, err := blem.ReadRobotPartKey()
			if err != nil {
				bLogger.Errorf("failed to read robot part key: %s", v)
				return
			}
			if v != "" {
				robotPartKey = v
				return
			}
		}
	}, wg.Done)

	// At this point we've received all required credentials (and can stop advertising)
	wg.Wait()
	bLogger.Infof("SSID: %s, Passkey: %s, Robot Part Key ID: %s, Robot Part Key: %s",
		ssid, psk, robotPartKeyID, robotPartKey)
	if err := blem.StopAdvertising(); err != nil {
		bLogger.Fatalf("failed to stop advertising characteristics in BLE: %v", err)
	}

	// Once Wi-Fi credentials are transmitted over bluetooth, prepare
	// network manager for Wi-Fi connection.
	wLogger := golog.NewDebugLogger("Wi-Fi manager")
	lwf, err := wf.NewLinuxWiFiManager(ctx, wLogger)
	if err != nil {
		wLogger.Fatalf("failed to set up Wi-Fi manager: %v", err)
	}
	if err := lwf.ConnectToWiFi(ctx, ssid, psk); err != nil {
		wLogger.Fatalf("failed to connect to Wi-Fi: %v", err)
	}
}
