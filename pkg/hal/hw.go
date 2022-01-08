package hal

import "github.com/tarm/serial"

type ChipMode int

type OnMessageCb func([]byte, error)

const (
	ModeNormal ChipMode = iota
	ModeWakeUp
	ModePowerSave
	ModeSleep
)

type HWHandler interface {
	ReadSerial() ([]byte, error)
	WriteSerial(msg []byte) error
	StageSerialPortConfig(baudRate int, parityBit serial.Parity)
	SetChipMode(mode ChipMode) error
	GetChipMode() (ChipMode, error)
	RegisterOnMessageCb(OnMessageCb) error
}
