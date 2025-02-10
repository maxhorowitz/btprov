package bleperipheral

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/pkg/errors"
	"go.viam.com/utils"

	"github.com/edaniels/golog"
	"github.com/google/uuid"
	"tinygo.org/x/bluetooth"
)

type BLEPeripheral interface {
	StartAdvertising(context.Context) error
	StopAdvertising() error

	ReadSsid() (string, error)
	ReadPsk() (string, error)
	ReadRobotPartKeyID() (string, error)
	ReadRobotPartKey() (string, error)
}

type availableWiFiNetworks struct {
	Networks []*availableWiFiNetwork `json:"networks"`
}

func (awns *availableWiFiNetworks) ToBytes() ([]byte, error) {
	return json.Marshal(awns)
}

type availableWiFiNetwork struct {
	Ssid        string  `json:"ssid"`
	Strength    float64 `json:"strength"` // This float, in the inclusive range [0.0, 1.0], represents the % strength of a WiFi network.
	RequiresPsk bool    `json:"requires_psk"`
}

func (awn *availableWiFiNetwork) ToBytes() ([]byte, error) {
	return json.Marshal(awn)
}

func NewAvailableWiFiNetwork(ssid string, strength float64, requiresPsk bool) (*availableWiFiNetwork, error) {
	if ssid == "" {
		return nil, errors.New("must provide non-empty ssid")
	}
	if strength == 0 {
		return nil, errors.New("must provide strength greater than zero")
	}
	return &availableWiFiNetwork{
		Ssid:        ssid,
		Strength:    strength,
		RequiresPsk: requiresPsk,
	}, nil
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

	characteristicSsid                  *linuxBLECharacteristic[*string]
	characteristicPsk                   *linuxBLECharacteristic[*string]
	characteristicRobotPartKeyID        *linuxBLECharacteristic[*string]
	characteristicRobotPartKey          *linuxBLECharacteristic[*string]
	characteristicAvailableWiFiNetworks *linuxBLECharacteristic[*string]
}

func NewLinuxBLEPeripheral(_ context.Context, logger golog.Logger, name string, awns ...availableWiFiNetwork) (BLEPeripheral, error) {
	if err := validateSystem(logger); err != nil {
		return nil, errors.WithMessage(err, "cannot initialize bluetooth peripheral, system requisites not met")
	}

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
	charAvailableWiFiNetworksUUID := bluetooth.NewUUID(uuid.New()).Replace16BitComponent(0x6666)
	logger.Infof("charAvailableWiFiNetworksUUID: %s", charAvailableWiFiNetworksUUID.String())

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
	charAvailableWiFiNetworks := &linuxBLECharacteristic[*string]{
		UUID:   charAvailableWiFiNetworksUUID,
		mu:     &sync.Mutex{},
		active: true,
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

	// Instantiate a read-only characteristic for broadcasting nearby, available WiFi networks.
	networks := make([]*availableWiFiNetwork, len(awns))
	for i, awn := range awns {
		networks[i] = &awn
	}
	availableWiFiNetworks := &availableWiFiNetworks{
		Networks: networks,
	}
	availableWiFiNetworksBytes, err := availableWiFiNetworks.ToBytes()
	if err != nil {
		return nil, errors.WithMessage(err, "unable to marshal available networks into bytes")
	}
	charConfigAvailableWiFiNetworks := bluetooth.CharacteristicConfig{
		UUID:  charAvailableWiFiNetworksUUID,
		Flags: bluetooth.CharacteristicReadPermission,
		Value: availableWiFiNetworksBytes, // Fill this in with available WiFi networks.
	}

	// Create service which will advertise each of the above characteristics.
	s := &bluetooth.Service{
		UUID: serviceUUID,
		Characteristics: []bluetooth.CharacteristicConfig{
			charConfigSsid,
			charConfigPsk,
			charConfigRobotPartKeyID,
			charConfigRobotPartKey,
			charConfigAvailableWiFiNetworks,
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

		characteristicSsid:                  charSsid,
		characteristicPsk:                   charPsk,
		characteristicRobotPartKeyID:        charRobotPartKeyID,
		characteristicRobotPartKey:          charRobotPartKey,
		characteristicAvailableWiFiNetworks: charAvailableWiFiNetworks,
	}, nil
}

func (s *linuxBLEService) StartAdvertising(ctx context.Context) error {
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
	utils.ManagedGo(func() {
		if err := listenForPairing(s.logger); err != nil {
			s.logger.Errorw("failed to listen for pairing requests and automatically connect", "err", err)
		}
	}, nil)
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

type ErrBLECharNoValue struct {
	missingValue string
}

func (e *ErrBLECharNoValue) Error() string {
	return fmt.Sprintf("No value has been written to BLE characteristic for %s", e.missingValue)
}

func newErrBLECharNoValue(missingValue string) error {
	return &ErrBLECharNoValue{
		missingValue: missingValue,
	}
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
		return "", newErrBLECharNoValue("ssid")
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
		return "", newErrBLECharNoValue("psk")
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
		return "", newErrBLECharNoValue("robot part key ID")
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
		return "", newErrBLECharNoValue("robot part key")
	}
	return *s.characteristicRobotPartKey.currentValue, nil
}
