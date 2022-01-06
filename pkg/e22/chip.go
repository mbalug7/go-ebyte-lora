package e22

import (
	"fmt"
	"log"
	"time"

	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
	"github.com/tarm/serial"
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
	registers [8]Register
	hw        *hal.ChipHWHandler
}

func NewChip(hwHanlder *hal.ChipHWHandler) (*Chip, error) {
	ch := &Chip{
		// ordered array, AddH address is 0, CryptL address is 7
		hw: hwHanlder,
		registers: [8]Register{
			&AddH{},
			&AddL{},
			&Reg0{},
			&Reg1{},
			&Reg2{},
			&Reg3{},
			&CryptH{},
			&CryptL{},
		},
	}
	data, err := ch.ReadRegisters(0x00, 0x06)
	if err != nil {
		return nil, err
	}
	err = ch.SaveConfig(data)
	if err != nil {
		return nil, err
	}
	err = ch.updateSerialStreamConfig()
	return ch, err
}

func (obj *Chip) ReadRegisters(startingAddress uint8, length uint8) (data []byte, err error) {

	err = obj.hw.SetChipMode(hal.ModeSleep)
	if err != nil {
		return data, fmt.Errorf("failed to set chip mode in get config %s", err.Error())
	}

	err = obj.hw.WriteSerial([]byte{0xC1, startingAddress, length})
	if err != nil {
		return data, fmt.Errorf("failed to write get config bytes %s", err.Error())
	}
	time.Sleep(200 * time.Millisecond)
	data, err = obj.hw.ReadSerial()
	if err != nil {
		return data, fmt.Errorf("failed to read config from serial %s", err.Error())
	}

	return
}

func (obj *Chip) SaveConfig(data []byte) error {
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

	for ind, reg := range obj.registers {
		log.Printf("ADDR %d, struct %+v \n", ind, reg)
	}
	return nil
}

func (obj *Chip) WriteConfigToChip(temporaryConfig bool, stagedRegisters [8]Register, nextChipMode hal.ChipMode) error {
	err := obj.hw.SetChipMode(hal.ModeSleep)
	if err != nil {
		return fmt.Errorf("failed to start config builder %s", err.Error())
	}
	var data []byte
	//  don't write crypt bytes if not set
	if stagedRegisters[CRYPT_H].(*CryptH).value != 0 || stagedRegisters[CRYPT_L].(*CryptL).value != 0 {
		data = make([]byte, 11)
		data[2] = 8 // params length
	} else {
		data = make([]byte, 9)
		data[2] = 6 // params length
	}
	data[0] = 0xC0
	if temporaryConfig {
		data[0] = 0xC2
	}
	data[1] = 0x00 // start from zero

	for i := 3; i < len(data); i++ {
		data[i] = stagedRegisters[i-3].GetValue()
	}
	err = obj.hw.WriteSerial(data)
	if err != nil {
		return fmt.Errorf("failed to write config to the chip: %s", err.Error())
	}
	time.Sleep(200 * time.Millisecond)
	chipCfg, err := obj.hw.ReadSerial()
	if err != nil {
		return fmt.Errorf("failed to receive set config response: %s", err.Error())
	}
	err = obj.SaveConfig(chipCfg)
	if err != nil {
		return fmt.Errorf("failed to save chip config to lib model: %s", err.Error())
	}

	err = obj.updateSerialStreamConfig()
	if err != nil {
		return fmt.Errorf("failed to update serial port config with a new data %s", err.Error())
	}

	err = obj.hw.SetChipMode(nextChipMode)
	if err != nil {
		return fmt.Errorf("failed to set nextchip mode %s", err.Error())
	}
	return nil
}

func (obj *Chip) updateSerialStreamConfig() error {
	reg0 := obj.registers[REG0].(*Reg0)
	baud := serialBaudMap[reg0.baudRate]
	parity := serialParityMap[reg0.parityBit]
	obj.hw.StageSerialPortConfig(baud, parity)
	return nil
}
