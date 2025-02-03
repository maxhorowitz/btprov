package blemanager

import (
	"context"
	"sync"
	"time"

	"github.com/edaniels/golog"
	"github.com/pkg/errors"
	"go.uber.org/multierr"
	"go.viam.com/utils"

	bp "github.com/maxhorowitz/btprov/ble/peripheral"
)

// credentials represents the minimum required information needed to provision a Viam Agent.
type credentials struct {
	ssid           string
	psk            string
	robotPartKeyID string
	robotPartKey   string
}

// GetSSID returns the SSID from a set of credentials.
func (c *credentials) GetSSID() string {
	return c.ssid
}

// GetPSK returns the passkey from a set of credentials.
func (c *credentials) GetPsk() string {
	return c.psk
}

// GetRobotPartKeyID returns the robot part key ID from a set of credentials.
func (c *credentials) GetRobotPartKeyID() string {
	return c.robotPartKeyID
}

// GetRobotPartKey returns the robot part key from a set of credentials.
func (c *credentials) GetRobotPartKey() string {
	return c.robotPartKey
}

// WaitForCredentials returns credentials which represent the information required to provision a robot part and its WiFi.
func WaitForCredentials(ctx context.Context, logger golog.Logger, blep bp.BLEPeripheral) (*credentials, error) {
	var ssid, psk, robotPartKeyID, robotPartKey string
	var ssidErr, pskErr, robotPartKeyIDErr, robotPartKeyErr error

	wg := &sync.WaitGroup{}
	wg.Add(4)
	utils.ManagedGo(
		func() {
			ssid, ssidErr = waitForBLEValue(ctx, blep.ReadSsid, "ssid")
		},
		wg.Done,
	)
	utils.ManagedGo(
		func() {
			psk, pskErr = waitForBLEValue(ctx, blep.ReadPsk, "psk")
		},
		wg.Done,
	)
	utils.ManagedGo(
		func() {
			robotPartKeyID, robotPartKeyIDErr = waitForBLEValue(ctx, blep.ReadRobotPartKeyID, "robot part key ID")
		},
		wg.Done,
	)
	utils.ManagedGo(
		func() {
			robotPartKey, robotPartKeyErr = waitForBLEValue(ctx, blep.ReadRobotPartKey, "robot part key")
		},
		wg.Done,
	)
	wg.Wait()

	return &credentials{
		ssid: ssid, psk: psk, robotPartKeyID: robotPartKeyID, robotPartKey: robotPartKey,
	}, multierr.Combine(ssidErr, pskErr, robotPartKeyIDErr, robotPartKeyErr)
}

// waitForBLE is used to check for the existence of a new value in a BLE characteristic.
func waitForBLEValue(
	ctx context.Context, fn func() (string, error), description string,
) (string, error) {
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			time.Sleep(time.Second)
		}
		v, err := fn()
		if err != nil {
			var errBLECharNoValue *bp.ErrBLECharNoValue
			if errors.As(err, &errBLECharNoValue) {
				continue
			}
			return "", errors.WithMessagef(err, "failed to read %s", description)
		}
		return v, nil
	}
}
