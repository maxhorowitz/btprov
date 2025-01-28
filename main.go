package main

import (
	"fmt"

	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

func main() {
	// Enable the Bluetooth adapter
	err := adapter.Enable()
	if err != nil {
		panic(fmt.Sprintf("Failed to enable adapter: %v", err))
	}

	// Define the service UUID
	serviceUUID := bluetooth.NewUUID([16]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf1})

	// Create the service
	service := &bluetooth.Service{
		UUID: serviceUUID,
	}

	// Add the service to the adapter
	err = adapter.AddService(service)
	if err != nil {
		panic(fmt.Sprintf("Failed to add service: %v", err))
	}

	// Define the characteristic UUID
	charUUID := bluetooth.NewUUID([16]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf2})

	// Create a characteristic
	characteristicConfig := bluetooth.CharacteristicConfig{
		UUID:  charUUID,
		Flags: bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicWritePermission,
		WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
			fmt.Printf("Received data: %s\n", string(value))
		},
	}

	// Add the characteristic to the service
	// err = adapter.AddCharacteristic(service, &char)
	service.Characteristics = append(service.Characteristics, characteristicConfig)
	if err != nil {
		panic(fmt.Sprintf("Failed to add characteristic: %v", err))
	}

	// Start advertising the service
	if err := adapter.Enable(); err != nil {
		panic(fmt.Sprintf("Failed to enable the adapter: %v", err))
	}
	adv := adapter.DefaultAdvertisement()
	if adv == nil {
		panic("default advertisement is nil")
	}
	if err := adv.Configure(bluetooth.AdvertisementOptions{
		LocalName:    "Max Horowitz Raspberry Pi5",
		ServiceUUIDs: []bluetooth.UUID{charUUID},
	}); err != nil {
		panic("failed to configure bluetooth advertisement")
	}
	if err := adv.Start(); err != nil {
		panic("failed to start advertising")
	}
	fmt.Println("Bluetooth service is running...")
	select {} // Block forever
}
