package l3gd

import (
	"github.com/dasfoo/i2c"
	"github.com/golang/geo/r3"
)

// L3GD is a sensor driver implementation for L3GD20H Gyro.
// Documentation: http://goo.gl/Nb95rx
// Arduino code samples: https://github.com/pololu/l3g-arduino
type L3GD struct {
	bus     *i2c.Bus
	address byte
}

// DefaultAddress is a default I2C address for this sensor.
const DefaultAddress = 0x6b

// NewL3GD creates new instance of L3GD bound to I2C bus and address.
func NewL3GD(bus *i2c.Bus, addr byte) *L3GD {
	return &L3GD{
		bus:     bus,
		address: addr,
	}
}

const (
	regCtrl1  = 0x20
	regCtrl4  = 0x23
	regLowOdr = 0x39
)

// DataAvailabilityError is a "soft" error which tells that some data was
// either lost (not read by the user before it was overwritten with a new value),
// or not available yet (the measurement frequency is too low).
type DataAvailabilityError struct {
	NewDataNotAvailable   bool
	NewDataWasOverwritten bool
}

// Error returns human-readable description string for the error.
func (e *DataAvailabilityError) Error() string {
	if e.NewDataNotAvailable {
		return "Warning: there was no new measurement since the previous read."
	}
	if e.NewDataWasOverwritten {
		return "Warning: a new measurement was acquired before the previous was read."
	}
	return "An unknown error has occured. Data may be stale."
}

// Sleep puts the sensor in low power consumption mode.
func (l3g *L3GD) Sleep() error {
	// There's not just power control in CTRL1, we need to keep other values.
	var bw byte
	var err error
	if bw, err = l3g.bus.ReadByteFromReg(l3g.address, regCtrl1); err != nil {
		return err
	}
	// We are actually setting it to power-down mode rather than sleep.
	// Power-down consumes less power, but takes longer to wake.
	return l3g.bus.WriteByteToReg(l3g.address, regCtrl1, bw&^(1<<3))
}

// Wake enables sensor if it was put into power-down mode with Sleep().
func (l3g *L3GD) Wake() error {
	var bw byte
	var err error
	if bw, err = l3g.bus.ReadByteFromReg(l3g.address, regCtrl1); err != nil {
		return err
	}
	return l3g.bus.WriteByteToReg(l3g.address, regCtrl1, bw|0xf0)
}

var bitsLowodrDrForFrequency = [...][3]int{
	{12, 1, 0x0f},
	{25, 1, 0x1f},
	{50, 1, 0x2f},
	{100, 0, 0x0f},
	{200, 0, 0x1f},
	{400, 0, 0x2f},
	{800, 0, 0x3f},
}

// SetFrequency sets Output Data Rate, in Hz, range 12 .. 800.
func (l3g *L3GD) SetFrequency(hz int) error {
	// ~250 dps full scale (gain).
	if err := l3g.bus.WriteByteToReg(l3g.address, regCtrl4, 0x00); err != nil {
		return err
	}
	for i := 0; i < len(bitsLowodrDrForFrequency); i++ {
		if bitsLowodrDrForFrequency[i][0] >= hz || i == len(bitsLowodrDrForFrequency)-1 {
			if err := l3g.bus.WriteByteToReg(l3g.address, regLowOdr,
				byte(bitsLowodrDrForFrequency[i][1])); err != nil {
				return err
			}
			return l3g.bus.WriteByteToReg(l3g.address, regCtrl1,
				byte(bitsLowodrDrForFrequency[i][2]))
		}
	}
	// This should never happen.
	return nil
}

// Read reads new data from the sensor.
// Note: err might be a warning about data "freshness" if it's DataAvailabilityError.
// Call sequence:
//   SetFrequency(...)
//   in a loop: Read()
func (l3g *L3GD) Read() (v r3.Vector, err error) {
	bytes := make([]byte, 7)
	if _, err = l3g.bus.ReadSliceFromReg(l3g.address, 0x27|(1<<7), bytes); err != nil {
		return
	}
	// Terrible casts, but what could we do?
	v.X = float64((int(int8(bytes[2])) << 8) | int(int8(bytes[1])))
	v.Y = float64((int(int8(bytes[4])) << 8) | int(int8(bytes[3])))
	v.Z = float64((int(int8(bytes[6])) << 8) | int(int8(bytes[5])))
	if bytes[0]&0xf0 > 0 {
		err = &DataAvailabilityError{NewDataWasOverwritten: true}
	} else if bytes[0]&0x0f == 0 {
		err = &DataAvailabilityError{NewDataNotAvailable: true}
	}
	return
}