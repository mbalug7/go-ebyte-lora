package hal

import "github.com/tarm/serial"

// ChipMode defines chip mode type that is used across the lib
type ChipMode int

// OnMessageCb registers callback method that is called when a new message is received
type OnMessageCb func([]byte, error)

// chip modes, read module documentation for more info
const (
	ModeNormal ChipMode = iota
	ModeWakeUp
	ModePowerSave
	ModeSleep
)

// HWHandler interface that defines module handler -> handler that is used to communicate and control eByte lora module
type HWHandler interface {
	ReadSerial() ([]byte, error)
	WriteSerial(msg []byte) error
	StageSerialPortConfig(baudRate int, parityBit serial.Parity)
	SetMode(mode ChipMode) error
	GetMode() (ChipMode, error)
	RegisterOnMessageCb(OnMessageCb) error
}
