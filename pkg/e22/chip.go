package e22

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
	"github.com/tarm/serial"
)

type Message struct {
	Payload []byte
	RSSI    uint8
}

type OnMessageCb func(Message, error)

type command uint8

func (obj command) toByte() byte {
	return byte(obj)
}

const (
	cmdSetRegPermanent command = 0xC0
	cmdGetReg          command = 0xC1
	cmdSetRegTemporary command = 0xC2
)

var serialBaudMap = map[baudRate]int{
	BAUD_1200:   1200,
	BAUD_2400:   2400,
	BAUD_4800:   4800,
	BAUD_9600:   9600,
	BAUD_19200:  19200,
	BAUD_38400:  38400,
	BAUD_57600:  57600,
	BAUD_115200: 115200,
}

var serialParityMap = map[parity]serial.Parity{
	PARITY_8N1: serial.ParityNone,
	PARITY_8O1: serial.ParityOdd,
	PARITY_8E1: serial.ParityEven,
}

type Chip struct {
	registers registersCollection
	hw        hal.HWHandler
	onMsgCb   OnMessageCb
}

func NewChip(gpioHandler hal.HWHandler, cb OnMessageCb) (*Chip, error) {
	mode, err := gpioHandler.GetChipMode()
	if err != nil {
		return nil, fmt.Errorf("failed to get chip mode: %w", err)
	}
	ch := &Chip{
		hw:        gpioHandler,
		registers: newRegistersCollection(),
		onMsgCb:   cb,
	}
	err = gpioHandler.RegisterOnMessageCb(ch.onMessageHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to register OnMessageCb: %w", err)
	}
	// E22 chip, first six registers are readable
	data, err := ch.readChipRegisters(0x00, 0x06)
	if err != nil {
		return nil, err
	}
	err = ch.saveConfig(data)
	if err != nil {
		return nil, err
	}
	err = ch.updateSerialStreamConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to update serial port config with the baud and parity values that are stored on chip: %w", err)
	}
	err = ch.hw.SetChipMode(mode)
	if err != nil {
		return nil, fmt.Errorf("failed to set chip mode: %w", err)
	}
	return ch, err
}

func (obj *Chip) onMessageHandler(msg []byte, err error) {
	if err != nil {
		if errors.Is(err, io.EOF) {
			return
		}
		obj.onMsgCb(Message{}, err)
	}
	if obj.registers[REG3].(*Reg3).enableRSSI == RSSI_ENABLE {
		if len(msg) < 2 {
			obj.onMsgCb(Message{}, fmt.Errorf("invalid message received"))
			return
		}
		obj.onMsgCb(
			Message{
				Payload: msg[0 : len(msg)-1],
				RSSI:    msg[len(msg)-1],
			},
			err,
		)
	} else {
		obj.onMsgCb(Message{Payload: msg, RSSI: 0}, err)
	}
}

func (obj *Chip) readChipRegisters(startingAddress hal.RegAddress, length uint8) (data []byte, err error) {

	err = obj.hw.SetChipMode(hal.ModeSleep)
	if err != nil {
		return data, fmt.Errorf("failed to set chip mode in get config: %w", err)
	}

	err = obj.hw.WriteSerial([]byte{cmdGetReg.toByte(), startingAddress.ToByte(), length})
	if err != nil {
		return data, fmt.Errorf("failed to write get config bytes: %w", err)
	}
	time.Sleep(200 * time.Millisecond)
	data, err = obj.hw.ReadSerial()
	if err != nil {
		return data, fmt.Errorf("failed to read config from serial: %w", err)
	}
	return
}

func (obj *Chip) saveConfig(data []byte) error {
	// we ned at least 3 bytes, cmd, start address, length
	if len(data) < 3 {
		return fmt.Errorf("invalid config")
	}
	startAddr := data[1]
	length := data[2]

	// cmd, starting address, length, and parameters -> number of parameters must be the same as length
	// one parameter for one register
	// e.g. C1 04 01 09 -> (len(conf) is 4) < (3 -> first parameter postion) + (conf[2] is 1 -> we have only one parameter) -> 4 is not < 4
	if len(data) < 3+int(length) {
		return fmt.Errorf("invalid parameters in config")
	}
	paramStartPosition := 3
	for i := startAddr; i < startAddr+length; i++ {
		obj.registers[i].SetValue(data[paramStartPosition])
		paramStartPosition++
	}
	return nil
}

func (obj *Chip) WriteConfigToChip(temporaryConfig bool, stagedRegisters registersCollection) error {
	if stagedRegisters.EqualTo(obj.registers) {
		return fmt.Errorf("new register setup is the same as the setup on the chip, ignoring")
	}
	currentMode, err := obj.hw.GetChipMode()
	if err != nil {
		return fmt.Errorf("failed to get current chip mode: %w", err)
	}
	err = obj.hw.SetChipMode(hal.ModeSleep)
	if err != nil {
		return fmt.Errorf("failed to start config builder: %w", err)
	}
	serialDataLen := uint8(11)
	//  don't write crypt bytes if not set in new config
	if stagedRegisters[CRYPT_H].(*CryptH).value == 0 && stagedRegisters[CRYPT_L].(*CryptL).value == 0 {
		serialDataLen = 9
	}

	data := make([]byte, serialDataLen)
	data[0] = cmdSetRegPermanent.toByte()
	if temporaryConfig {
		data[0] = cmdSetRegTemporary.toByte()
	}
	data[1] = ADD_H.ToByte() // start from te first register

	data[2] = serialDataLen - 3 // data[2] defines param length

	// start from 3, because parameters list starts after cmd, startingAddress, length, and parameters
	for i := 3; i < len(data); i++ {
		data[i] = stagedRegisters[i-3].GetValue()
	}
	err = obj.hw.WriteSerial(data)
	if err != nil {
		return fmt.Errorf("failed to write config to the chip: %w", err)
	}
	time.Sleep(200 * time.Millisecond)
	chipCfg, err := obj.hw.ReadSerial()
	if err != nil {
		return fmt.Errorf("failed to receive set config response: %w", err)
	}
	err = obj.saveConfig(chipCfg)
	if err != nil {
		return fmt.Errorf("failed to save chip config to lib model: %w", err)
	}

	err = obj.updateSerialStreamConfig()
	if err != nil {
		return fmt.Errorf("failed to update serial port config with the new data: %w", err)
	}

	err = obj.hw.SetChipMode(currentMode)
	if err != nil {
		return fmt.Errorf("failed to set nextchip mode %w", err)
	}
	return nil
}

func (obj *Chip) SendMessage(message string) error {
	currentMode, err := obj.hw.GetChipMode()
	if err != nil {
		return err
	}
	if currentMode == hal.ModeSleep || currentMode == hal.ModePowerSave {
		return fmt.Errorf("can't send message while chip is in mode %d. Change mode to ModeNormal or ModeWakeUp", currentMode)
	}
	err = obj.hw.WriteSerial([]byte(message))
	if err != nil {
		return fmt.Errorf("failed to write config to the chip: %w", err)
	}
	return nil
}

func (obj *Chip) GetModuleConfiguration() string {
	var conf string
	for _, reg := range obj.registers {
		conf = conf + fmt.Sprintf("\nREG [%d]: %+v", reg.GetAddress(), reg)
	}
	return conf
}

func (obj *Chip) updateSerialStreamConfig() error {
	// get chip serial config and apply it to the serial port handler
	reg0 := obj.registers[REG0].(*Reg0)
	baud := serialBaudMap[reg0.baudRate]
	parity := serialParityMap[reg0.parityBit]
	obj.hw.StageSerialPortConfig(baud, parity)
	return nil
}
