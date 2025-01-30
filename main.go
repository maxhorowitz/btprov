package main

import (
	"context"
	"log"

	"github.com/edaniels/golog"
	pm "github.com/maxhorowitz/btprov/provisioning_manager"
	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

func main() {
	ctx := context.Background()
	logger := golog.NewDebugLogger("provisioning")

	// Once Wi-Fi credentials are transmitted over bluetooth, prepare
	// network manager for Wi-Fi connection.
	lpm, err := pm.NewProvisioningManager(ctx, logger)
	if err != nil {
		log.Fatalf("failure setting up provisioning manager: %v", err)
	}
	if err := lpm.ConnectToWiFi(ctx, logger, "Viam", "checkmate"); err != nil {
		log.Fatalf("failure connection to Wi-Fi: %v", err)
	}
}

// func main2() {
// 	// Enable the Bluetooth adapter
// 	err := adapter.Enable()
// 	if err != nil {
// 		panic(fmt.Sprintf("Failed to enable adapter: %v", err))
// 	}
// 	// Generate BLE UUIDs
// 	serviceUUID := bluetooth.NewUUID(uuid.New())
// 	charUUIDReadWrite := bluetooth.NewUUID(uuid.New())
// 	charUUIDWriteOnly := bluetooth.NewUUID(uuid.New())

// 	// Convert UUIDs to string format
// 	serviceUUIDStr := serviceUUID.String()
// 	charUUIDReadWriteStr := charUUIDReadWrite.String()
// 	charUUIDWriteOnlyStr := charUUIDWriteOnly.String()

// 	// Print the UUIDs with labels
// 	fmt.Println("Generated BLE UUIDs:")
// 	fmt.Println("Service UUID         :", serviceUUIDStr)
// 	fmt.Println("Characteristic 1 UUID:", charUUIDReadWriteStr)
// 	fmt.Println("Characteristic 2 UUID:", charUUIDWriteOnlyStr)

// 	// Create a read and write characteristic
// 	characteristicConfigReadWrite := bluetooth.CharacteristicConfig{
// 		UUID:  charUUIDReadWrite,
// 		Flags: bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicWritePermission | bluetooth.CharacteristicBroadcastPermission,
// 		WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
// 			fmt.Printf("Received data: %s\n", string(value))
// 		},
// 	}

// 	// Create a write only characteristic
// 	characteristicConfigWriteOnly := bluetooth.CharacteristicConfig{
// 		UUID:  charUUIDWriteOnly,
// 		Flags: bluetooth.CharacteristicWritePermission,
// 		WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
// 			fmt.Printf("Received data: %s\n", string(value))
// 		},
// 	}

// 	// Create the service
// 	service := &bluetooth.Service{
// 		UUID: serviceUUID,
// 		Characteristics: []bluetooth.CharacteristicConfig{
// 			characteristicConfigReadWrite,
// 			characteristicConfigWriteOnly,
// 		},
// 	}

// 	// Add the service to the adapter
// 	err = adapter.AddService(service)
// 	if err != nil {
// 		panic(fmt.Sprintf("Failed to add service: %v", err))
// 	}

// 	// Start advertising the service
// 	if err := adapter.Enable(); err != nil {
// 		panic(fmt.Sprintf("Failed to enable the adapter: %v", err))
// 	}
// 	adv := adapter.DefaultAdvertisement()
// 	if adv == nil {
// 		panic("default advertisement is nil")
// 	}
// 	if err := adv.Configure(bluetooth.AdvertisementOptions{
// 		LocalName:    "Max Horowitz II Raspberry Pi5",
// 		ServiceUUIDs: []bluetooth.UUID{serviceUUID},
// 	}); err != nil {
// 		panic("failed to configure bluetooth advertisement")
// 	}
// 	if err := adv.Start(); err != nil {
// 		panic("failed to start advertising")
// 	}
// 	fmt.Println("Bluetooth service is running...")
// 	select {} // Block forever
// }
