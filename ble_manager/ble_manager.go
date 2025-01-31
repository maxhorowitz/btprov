package blemanager

import (
	"context"
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

type linuxBLECharacteristic[T any] struct {
	UUID   bluetooth.UUID
	mu     *sync.Mutex
	active bool // Currently non-functional, but should be used to make characteristics optional.

	currentValue T
}

type linuxBLEService struct {
	logger golog.Logger
	mu     *sync.Mutex

	adv       *bluetooth.Advertisement
	advActive bool
	UUID      bluetooth.UUID

	characteristicSsid           *linuxBLECharacteristic[*string]
	characteristicPsk            *linuxBLECharacteristic[*string]
	characteristicRobotPartKeyID *linuxBLECharacteristic[*string]
	characteristicRobotPartKey   *linuxBLECharacteristic[*string]
}

func NewLinuxBLEPeripheral(_ context.Context, logger golog.Logger, name string) (BLEPeripheral, error) {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return nil, errors.WithMessage(err, "failed to enable bluetooth adapter")
	}

	serviceUUID := bluetooth.NewUUID(uuid.New()).Replace16BitComponent(0x1111)
	logger.Infof("serviceUUID: %s", serviceUUID.String())
	charSsidUUID := bluetooth.NewUUID(uuid.New()).Replace16BitComponent(0x2222)
	logger.Infof("charSsidUUID: %s", charSsidUUID.String())
	charPskUUID := bluetooth.NewUUID(uuid.New()).Replace16BitComponent(0x3333)
	logger.Infof("charPskUUID: %s", charPskUUID.String())
	charRobotPartKeyIDUUID := bluetooth.NewUUID(uuid.New()).Replace16BitComponent(0x4444)
	logger.Infof("charRobotPartKeyIDUUID: %s", charRobotPartKeyIDUUID.String())
	charRobotPartKeyUUID := bluetooth.NewUUID(uuid.New()).Replace16BitComponent(0x5555)
	logger.Infof("charRobotPartKeyUUID: %s", charRobotPartKeyUUID.String())

	// Create abstracted characteristics which act as a buffer for reading data from bluetooth.
	charSsid := &linuxBLECharacteristic[*string]{
		UUID:         charSsidUUID,
		mu:           &sync.Mutex{},
		active:       true,
		currentValue: nil,
	}
	charPsk := &linuxBLECharacteristic[*string]{
		UUID:         charPskUUID,
		mu:           &sync.Mutex{},
		active:       true,
		currentValue: nil,
	}
	charRobotPartKeyID := &linuxBLECharacteristic[*string]{
		UUID:         charRobotPartKeyIDUUID,
		mu:           &sync.Mutex{},
		active:       true,
		currentValue: nil,
	}
	charRobotPartKey := &linuxBLECharacteristic[*string]{
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
	return &linuxBLEService{
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

func (s *linuxBLEService) StartAdvertising() error {
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

func (s *linuxBLEService) StopAdvertising() error {
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

func (s *linuxBLEService) ReadSsid() (string, error) {
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

func (s *linuxBLEService) ReadPsk() (string, error) {
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

func (s *linuxBLEService) ReadRobotPartKeyID() (string, error) {
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

func (s *linuxBLEService) ReadRobotPartKey() (string, error) {
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
