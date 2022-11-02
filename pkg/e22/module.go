package e22

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
)

// Message struct that holds received data
type Message struct {
	Payload []byte
	RSSI    uint8
}

// OnMessageCb defines on message callback type
type OnMessageCb func(Message, error)

const (
	cmdSetRegPermanent byte = 0xC0
	cmdGetReg          byte = 0xC1
	cmdSetRegTemporary byte = 0xC2
)

// chipRsp defines module response structure
type chipRsp struct {
	command   byte
	startAddr byte
	length    byte
	params    []byte
}

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

var serialParityMap = map[parity]hal.Parity{
	PARITY_8N1: hal.ParityNone,
	PARITY_8O1: hal.ParityOdd,
	PARITY_8E1: hal.ParityEven,
}

// Module E22 module object
type Module struct {
	registers registersCollection
	hw        hal.HWHandler
	onMsgCb   OnMessageCb
}

// NewModule constract new E22 module, reads current configuration and sets chip mode
func NewModule(gpioHandler hal.HWHandler, cb OnMessageCb) (*Module, error) {
	mode, err := gpioHandler.GetMode()
	if err != nil {
		return nil, fmt.Errorf("failed to get chip mode: %w", err)
	}
	ch := &Module{
		hw:        gpioHandler,
		registers: newRegistersCollection(),
		onMsgCb:   cb,
	}
	err = gpioHandler.RegisterOnMessageCb(ch.onMessageHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to register OnMessageCb: %w", err)
	}
	// E22 module, first six registers are readable
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
	err = ch.hw.SetMode(mode)
	if err != nil {
		return nil, fmt.Errorf("failed to set chip mode: %w", err)
	}
	return ch, err
}

// onMessageHandler parses received message and construct human readable message
func (obj *Module) onMessageHandler(msg []byte, err error) {
	if err != nil {
		if errors.Is(err, io.EOF) {
			return
		}
		obj.onMsgCb(Message{}, err)
		return
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
		return
	}
	obj.onMsgCb(Message{Payload: msg, RSSI: 0}, err)
}

// readChipRegisters reads all the registers on the chip
func (obj *Module) readChipRegisters(startingAddress hal.RegAddress, length uint8) (data []byte, err error) {

	err = obj.hw.SetMode(hal.ModeSleep)
	if err != nil {
		return data, fmt.Errorf("failed to set chip mode in get config: %w", err)
	}

	err = obj.hw.WriteSerial([]byte{cmdGetReg, startingAddress.ToByte(), length})
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

// saveConfig updates lib internal cache with the real registers values on the module
func (obj *Module) saveConfig(data []byte) error {

	rsp, err := obj.parseChipResponse(data)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	obj.registers.Update(rsp.startAddr, rsp.params)
	return nil
}

// getConfigSetRequest returns byte array that holds registers data that must be set
// temporary construct temporary config that will be reset after chip reboot
// registers collection of register values that must be set on real module
func (obj *Module) getConfigSetRequest(temporary bool, registers registersCollection) []byte {

	params := registers[0:]
	if registers[CRYPT_H].(*CryptH).value == 0 && registers[CRYPT_L].(*CryptL).value == 0 {
		params = registers[0 : len(registers)-2]
	}
	const paramsStartPosition = 3
	serialDataLen := len(params) + paramsStartPosition
	//  don't write crypt bytes if not set in new config
	data := make([]byte, serialDataLen)
	data[0] = cmdSetRegPermanent
	if temporary {
		data[0] = cmdSetRegTemporary
	}
	data[1] = ADD_H.ToByte() // start from te first register

	data[2] = byte(len(params)) // data[2] defines param length

	// start from 3, because parameters list starts after cmd, startingAddress, and length bytes
	for i := 0; i < len(params); i++ {
		data[i+3] = params[i].GetValue()
	}
	return data
}

// parseChipResponse when the module is in config mode, it returns response that must be parsed, read datasheet
// returns new chipRsp object that holds parsed data
func (obj *Module) parseChipResponse(data []byte) (chipRsp, error) {

	if len(data) < 4 {
		return chipRsp{}, fmt.Errorf("invalid command")
	}
	startAddr := data[1]
	paramsNumber := data[2]
	params := data[3:]

	// if some random data are received after actual message, remove them
	if int(paramsNumber) > 0 {
		params = params[:paramsNumber]
	}

	if int(paramsNumber) != len(params) {
		return chipRsp{}, fmt.Errorf("invalid command, mismatch in length and params count, %x", data)
	}
	return chipRsp{
		command:   cmdGetReg,
		startAddr: startAddr,
		length:    paramsNumber,
		params:    params,
	}, nil
}

// updateSerialStreamConfig if the serial config that is stored on the module is different than current one,
// update serial config data
func (obj *Module) updateSerialStreamConfig() error {
	// get chip serial config and apply it to the serial port handler
	reg0 := obj.registers[REG0].(*Reg0)
	baud := serialBaudMap[reg0.baudRate]
	parity := serialParityMap[reg0.parityBit]
	obj.hw.StageSerialPortConfig(baud, parity)
	return nil
}

// WriteConfigToChip writes given config to module
func (obj *Module) WriteConfigToChip(temporaryConfig bool, stagedRegisters registersCollection) error {
	if stagedRegisters.EqualTo(obj.registers) {
		return fmt.Errorf("new register setup is the same as the setup on the chip, ignoring")
	}
	currentMode, err := obj.hw.GetMode()
	if err != nil {
		return fmt.Errorf("failed to get current chip mode: %w", err)
	}
	err = obj.hw.SetMode(hal.ModeSleep)
	if err != nil {
		return fmt.Errorf("failed to start config builder: %w", err)
	}
	data := obj.getConfigSetRequest(temporaryConfig, stagedRegisters)
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
	if !stagedRegisters.EqualTo(obj.registers) {
		return fmt.Errorf("current chip configuration is not the same as saved: %w", err)
	}

	err = obj.hw.SetMode(currentMode)
	if err != nil {
		return fmt.Errorf("failed to set nextchip mode %w", err)
	}
	return nil
}

// SendMessage sends given message to module via UART
func (obj *Module) SendMessage(message string) error {
	currentMode, err := obj.hw.GetMode()
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

// SendFixedMessage if you want to send message to some fixed address and channel, use this method
func (obj *Module) SendFixedMessage(addressHigh byte, addressLow byte, channel byte, message string) error {
	currentMode, err := obj.hw.GetMode()
	if err != nil {
		return err
	}
	if currentMode == hal.ModeSleep || currentMode == hal.ModePowerSave {
		return fmt.Errorf("can't send message while E22 module is in mode %d. Change the mode to ModeNormal or ModeWakeUp", currentMode)
	}
	if obj.registers[REG3].(*Reg3).transmissionMethod == TRANSMISSION_TRANSPARENT {
		return fmt.Errorf("can't send fixed message while module has TRANSMISSION_TRANSPARENT setup, reconfigure module to TRANSMISSION_FIXED mode")
	}
	msgBytes := []byte{addressHigh, addressLow, channel}
	msgBytes = append(msgBytes, []byte(message)...)

	err = obj.hw.WriteSerial(msgBytes)
	if err != nil {
		return fmt.Errorf("failed to write config to the chip: %w", err)
	}
	return nil
}

// GetModuleConfiguration returns human readable current module configuration
func (obj *Module) GetModuleConfiguration() string {
	var conf string
	for _, reg := range obj.registers {
		conf = conf + fmt.Sprintf("\nREG [%d]: %+v", reg.GetAddress(), reg)
	}
	return conf
}
