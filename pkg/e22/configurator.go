package e22

import (
	"fmt"
	"log"
	"time"

	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
)

type Chip struct {
	registers [8]Register
	hw        *hal.ChipHWHandler
}

func NewChip(hwHanlder *hal.ChipHWHandler) *Chip {
	return &Chip{
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
}

func (obj *Chip) GetConfig() (err error) {

	err = obj.hw.SetChipMode(hal.ModeSleep)
	if err != nil {
		return fmt.Errorf("failed to set chip mode in get config %s", err.Error())
	}

	err = obj.hw.WriteSerial([]byte{0xC1, 0x00, 0x06})
	if err != nil {
		return fmt.Errorf("failed to write get config bytes %s", err.Error())
	}
	time.Sleep(200 * time.Millisecond)
	data, err := obj.hw.ReadSerial()
	if err != nil {
		return fmt.Errorf("failed to read config from serial %s", err.Error())
	}
	err = obj.SetConfig(data)
	if err != nil {
		return fmt.Errorf("failed to store current chip new config %s", err.Error())
	}
	return nil
}

func (obj *Chip) SetConfig(conf []byte) error {
	// we ned at least 3 bytes, cmd, start address, length
	if len(conf) < 3 {
		return fmt.Errorf("invalid config")
	}
	startAddr := conf[1]
	length := conf[2]

	// cmd, starting address, length, and parameters -> number of parameters must be the same as length
	// one parameter for one register
	// e.g. C1 04 01 09 -> (len(conf) is 4) < (3 -> first parameter postion) + (conf[2] is 1 -> we have only one parameter) -> 4 is not < 4
	if len(conf) < 3+int(length) {
		return fmt.Errorf("invalid parameters in config")
	}
	paramStartPosition := 3
	for i := startAddr; i < startAddr+length; i++ {
		obj.registers[i].SetValue(conf[paramStartPosition])
		paramStartPosition++
	}

	for ind, reg := range obj.registers {
		log.Printf("ADDR %d, struct %+v \n", ind, reg)
	}
	return nil
}
