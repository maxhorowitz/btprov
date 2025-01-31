package blemanager

import (
	"context"
	"fmt"
	"sync"

	"github.com/pkg/errors"

	"github.com/edaniels/golog"
	"github.com/google/uuid"
	"tinygo.org/x/bluetooth"
)

type BLEPeripheral interface {
	StartAdvertising() error
	StopAdvertising() error

	ReadSsid() (string, error)
	ReadPsk() (string, error)
	ReadRobotPartKeyID() (string, error)
	ReadRobotPartKey() (string, error)
}

type characteristic[T any] struct {
	UUID   bluetooth.UUID
	mu     *sync.Mutex
	active bool // Currently non-functional, but should be used to make characteristics optional.

	currentValue T
}

type service struct {
	logger golog.Logger
	mu     *sync.Mutex

	adv       *bluetooth.Advertisement
	advActive bool
	UUID      bluetooth.UUID

	characteristicSsid           *characteristic[*string]
	characteristicPsk            *characteristic[*string]
	characteristicRobotPartKeyID *characteristic[*string]
	characteristicRobotPartKey   *characteristic[*string]
}

func NewLinuxBLEPeripheral(_ context.Context, logger golog.Logger, name string) (BLEPeripheral, error) {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return nil, errors.WithMessage(err, "failed to enable bluetooth adapter")
	}

	serviceUUID := bluetooth.NewUUID(uuid.New())
	charSsidUUID := bluetooth.NewUUID(uuid.New())
	charPskUUID := bluetooth.NewUUID(uuid.New())
	charRobotPartKeyIDUUID := bluetooth.NewUUID(uuid.New())
	charRobotPartKeyUUID := bluetooth.NewUUID(uuid.New())

	// Create abstracted characteristics which act as a buffer for reading data from bluetooth.
	charSsid := &characteristic[*string]{
		UUID:         charSsidUUID,
		mu:           &sync.Mutex{},
		active:       true,
		currentValue: nil,
	}
	charPsk := &characteristic[*string]{
		UUID:         charPskUUID,
		mu:           &sync.Mutex{},
		active:       true,
		currentValue: nil,
	}
	charRobotPartKeyID := &characteristic[*string]{
		UUID:         charRobotPartKeyIDUUID,
		mu:           &sync.Mutex{},
		active:       true,
		currentValue: nil,
	}
	charRobotPartKey := &characteristic[*string]{
		UUID:         charRobotPartKeyUUID,
		mu:           &sync.Mutex{},
		active:       true,
		currentValue: nil,
	}

	// Create write-only, mutexed characteristics (one per credential).
	charConfigSsid := bluetooth.CharacteristicConfig{
		UUID:  charSsidUUID,
		Flags: bluetooth.CharacteristicWritePermission,
		WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
			v := string(value)
			logger.Infof("Received SSID: %s", v)
			charSsid.mu.Lock()
			defer charSsid.mu.Unlock()
			charSsid.currentValue = &v
		},
	}
	charConfigPsk := bluetooth.CharacteristicConfig{
		UUID:  charPskUUID,
		Flags: bluetooth.CharacteristicWritePermission,
		WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
			v := string(value)
			logger.Infof("Received Passkey: %s", v)
			charPsk.mu.Lock()
			defer charPsk.mu.Unlock()
			charPsk.currentValue = &v
		},
	}
	charConfigRobotPartKeyID := bluetooth.CharacteristicConfig{
		UUID:  charRobotPartKeyIDUUID,
		Flags: bluetooth.CharacteristicWritePermission,
		WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
			v := string(value)
			logger.Infof("Received Robot Part Key ID: %s", v)
			charRobotPartKeyID.mu.Lock()
			defer charRobotPartKeyID.mu.Unlock()
			charRobotPartKeyID.currentValue = &v
		},
	}
	charConfigRobotPartKey := bluetooth.CharacteristicConfig{
		UUID:  charRobotPartKeyUUID,
		Flags: bluetooth.CharacteristicWritePermission,
		WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
			v := string(value)
			logger.Infof("Received Robot Part Key: %s", v)
			charRobotPartKey.mu.Lock()
			defer charRobotPartKey.mu.Unlock()
			charRobotPartKey.currentValue = &v
		},
	}

	// Create service which will advertise each of the above characteristics.
	s := &bluetooth.Service{
		UUID: serviceUUID,
		Characteristics: []bluetooth.CharacteristicConfig{
			charConfigSsid,
			charConfigPsk,
			charConfigRobotPartKeyID,
			charConfigRobotPartKey,
		},
	}
	if err := adapter.AddService(s); err != nil {
		return nil, errors.WithMessage(err, "unable to add bluetooth service to default adapter")
	}
	if err := adapter.Enable(); err != nil {
		return nil, errors.WithMessage(err, "failed to enable bluetooth adapter")
	}
	defaultAdvertisement := adapter.DefaultAdvertisement()
	if defaultAdvertisement == nil {
		return nil, errors.New("default advertisement is nil")
	}
	if err := defaultAdvertisement.Configure(
		bluetooth.AdvertisementOptions{
			LocalName:    name,
			ServiceUUIDs: []bluetooth.UUID{serviceUUID},
		},
	); err != nil {
		return nil, errors.WithMessage(err, "failed to configure default advertisement")
	}
	return &service{
		logger: logger,
		mu:     &sync.Mutex{},

		adv:       defaultAdvertisement,
		advActive: false,
		UUID:      serviceUUID,

		characteristicSsid:           charSsid,
		characteristicPsk:            charPsk,
		characteristicRobotPartKeyID: charRobotPartKeyID,
		characteristicRobotPartKey:   charRobotPartKey,
	}, nil
}

func (s *service) StartAdvertising() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.adv == nil {
		return errors.New("advertisement is nil")
	}
	if s.advActive {
		return errors.New("invalid request, advertising already active")
	}
	if err := s.adv.Start(); err != nil {
		return errors.WithMessage(err, "failed to start advertising")
	}
	s.advActive = true
	s.logger.Info("started advertising a BLE connection...")
	return nil
}

func (s *service) StopAdvertising() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.adv == nil {
		return errors.New("advertisement is nil")
	}
	if !s.advActive {
		return errors.New("invalid request, advertising already inactive")
	}
	if err := s.adv.Stop(); err != nil {
		return errors.WithMessage(err, "failed to stop advertising")
	}
	s.advActive = false
	s.logger.Info("stopped advertising a BLE connection")
	return nil
}

func (s *service) ReadSsid() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.characteristicSsid == nil {
		return "", errors.New("characteristic ssid is nil")
	}

	s.characteristicSsid.mu.Lock()
	defer s.characteristicSsid.mu.Unlock()

	if !s.characteristicSsid.active {
		return "", errors.New("characteristic ssid is inactive")
	}
	if s.characteristicSsid.currentValue == nil {
		return "", nil
	}
	return *s.characteristicSsid.currentValue, nil
}

func (s *service) ReadPsk() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.characteristicPsk == nil {
		return "", errors.New("characteristic psk is nil")
	}

	s.characteristicPsk.mu.Lock()
	defer s.characteristicPsk.mu.Unlock()

	if !s.characteristicPsk.active {
		return "", errors.New("characteristic psk is inactive")
	}
	if s.characteristicPsk.currentValue == nil {
		return "", nil
	}
	return *s.characteristicPsk.currentValue, nil
}

func (s *service) ReadRobotPartKeyID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.characteristicRobotPartKeyID == nil {
		return "", errors.New("characteristic robot part key ID is nil")
	}

	s.characteristicRobotPartKeyID.mu.Lock()
	defer s.characteristicRobotPartKeyID.mu.Unlock()

	if !s.characteristicRobotPartKeyID.active {
		return "", errors.New("characteristic robot part key ID is inactive")
	}
	if s.characteristicRobotPartKeyID.currentValue == nil {
		return "", nil
	}
	return *s.characteristicRobotPartKeyID.currentValue, nil
}

func (s *service) ReadRobotPartKey() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.characteristicRobotPartKey == nil {
		return "", errors.New("characteristic robot part key is nil")
	}

	s.characteristicRobotPartKey.mu.Lock()
	defer s.characteristicRobotPartKey.mu.Unlock()

	if !s.characteristicRobotPartKey.active {
		return "", errors.New("characteristic robot part key is inactive")
	}
	if s.characteristicRobotPartKey.currentValue == nil {
		return "", nil
	}
	return *s.characteristicRobotPartKey.currentValue, nil
}

func Old() {
	adapter := bluetooth.DefaultAdapter

	// Enable the Bluetooth adapter
	err := adapter.Enable()
	if err != nil {
		panic(fmt.Sprintf("Failed to enable adapter: %v", err))
	}
	// Generate BLE UUIDs
	serviceUUID := bluetooth.NewUUID(uuid.New())
	charUUIDReadWrite := bluetooth.NewUUID(uuid.New())
	charUUIDWriteOnly := bluetooth.NewUUID(uuid.New())

	// Convert UUIDs to string format
	serviceUUIDStr := serviceUUID.String()
	charUUIDReadWriteStr := charUUIDReadWrite.String()
	charUUIDWriteOnlyStr := charUUIDWriteOnly.String()

	// Print the UUIDs with labels
	fmt.Println("Generated BLE UUIDs:")
	fmt.Println("Service UUID         :", serviceUUIDStr)
	fmt.Println("Characteristic 1 UUID:", charUUIDReadWriteStr)
	fmt.Println("Characteristic 2 UUID:", charUUIDWriteOnlyStr)

	// Create a read and write characteristic
	characteristicConfigReadWrite := bluetooth.CharacteristicConfig{
		UUID:  charUUIDReadWrite,
		Flags: bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicWritePermission | bluetooth.CharacteristicBroadcastPermission,
		WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
			fmt.Printf("Received data: %s\n", string(value))
		},
	}

	// Create a write only characteristic
	characteristicConfigWriteOnly := bluetooth.CharacteristicConfig{
		UUID:  charUUIDWriteOnly,
		Flags: bluetooth.CharacteristicWritePermission,
		WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
			fmt.Printf("Received data: %s\n", string(value))
		},
	}

	// Create the service
	service := &bluetooth.Service{
		UUID: serviceUUID,
		Characteristics: []bluetooth.CharacteristicConfig{
			characteristicConfigReadWrite,
			characteristicConfigWriteOnly,
		},
	}

	// Add the service to the adapter
	err = adapter.AddService(service)
	if err != nil {
		panic(fmt.Sprintf("Failed to add service: %v", err))
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
		LocalName:    "Max Horowitz II Raspberry Pi5",
		ServiceUUIDs: []bluetooth.UUID{serviceUUID},
	}); err != nil {
		panic("failed to configure bluetooth advertisement")
	}
	if err := adv.Start(); err != nil {
		panic("failed to start advertising")
	}
	fmt.Println("Bluetooth service is running...")
	select {} // Block forever
}
