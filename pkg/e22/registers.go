package e22

import "github.com/mbalug7/go-ebyte-lora/pkg/hal"

type registersCollection [8]hal.Register

func newRegistersCollection() registersCollection {
	return registersCollection{
		&AddH{},
		&AddL{},
		&Reg0{},
		&Reg1{},
		&Reg2{},
		&Reg3{},
		&CryptH{},
		&CryptL{},
	}
}

const (
	ADD_H hal.RegAddress = iota
	ADD_L
	REG0
	REG1
	REG2
	REG3
	CRYPT_H
	CRYPT_L
)

// ADD_H specification

type AddH struct {
	address uint8
}

func (obj *AddH) GetAddress() hal.RegAddress {
	return ADD_H
}

func (obj *AddH) GetValue() uint8 {
	return obj.address
}

func (obj *AddH) SetValue(value uint8) {
	obj.address = value
}

// ADD_L specification

type AddL struct {
	address uint8
}

func (obj *AddL) GetAddress() hal.RegAddress {
	return ADD_L
}

func (obj *AddL) GetValue() uint8 {
	return obj.address
}

func (obj *AddL) SetValue(value uint8) {
	obj.address = value
}

// REG0 specification

type baudRate uint8

const (
	BAUD_1200   baudRate = 0x00
	BAUD_2400   baudRate = 0x20
	BAUD_4800   baudRate = 0x40
	BAUD_9600   baudRate = 0x60
	BAUD_19200  baudRate = 0x80
	BAUD_38400  baudRate = 0xA0
	BAUD_57600  baudRate = 0xC0
	BAUD_115200 baudRate = 0xE0
)

type parity uint8

const (
	PARITY_8N1 parity = 0x00
	PARITY_8O1 parity = 0x08
	PARITY_8E1 parity = 0x10
)

type airDataRate uint8

const (
	ADR_2400_0 airDataRate = iota
	ADR_2400_1
	ADR_2400
	ADR_4800
	ADR_9600
	ADR_19200
	ADR_38400
	ADR_62500
)

type Reg0 struct {
	baudRate  baudRate
	parityBit parity
	adRate    airDataRate
}

func (obj *Reg0) GetAddress() hal.RegAddress {
	return REG0
}

func (obj *Reg0) GetValue() uint8 {
	return uint8(obj.baudRate) | uint8(obj.parityBit) | uint8(obj.adRate)
}

func (obj *Reg0) SetValue(value uint8) {
	obj.baudRate = baudRate(value & 0xE0)  // get last 3 bits
	obj.parityBit = parity(value & 0x18)   // git bit 3 and 4
	obj.adRate = airDataRate(value & 0x07) // get first 3 bits
}

// REG1 specification

type subPacket uint8

const (
	BYTES_200 subPacket = 0x00
	BYTES_128 subPacket = 0x40
	BYTES_64  subPacket = 0x80
	BYTES_32  subPacket = 0xC0
)

type rssiAmbientNoise uint8

const (
	RSSI_AMBIENT_NOISE_DISABLE rssiAmbientNoise = 0x00
	RSSI_AMBIENT_NOISE_ENABLE  rssiAmbientNoise = 0x20
)

type transmittingPower uint8

const (
	TP_22_DBM transmittingPower = iota
	TP_17_DBM
	TP_13_DBM
	TP_10_DBM
)

type Reg1 struct {
	subPacket         subPacket
	ambientNoiseRSSI  rssiAmbientNoise
	transmittingPower transmittingPower
}

func (obj *Reg1) GetAddress() hal.RegAddress {
	return REG1
}

func (obj *Reg1) GetValue() uint8 {
	return uint8(obj.subPacket) | uint8(obj.ambientNoiseRSSI) | uint8(obj.transmittingPower)
}

func (obj *Reg1) SetValue(value uint8) {
	obj.subPacket = subPacket(value & 0xC0)
	obj.ambientNoiseRSSI = rssiAmbientNoise(value & 0x20)
	obj.transmittingPower = transmittingPower(value & 0x03)
}

// REG2 specification

// Actual frequency = 850.125 + CH *1M
type Reg2 struct {
	channel uint8 // 0-80 channels
}

func (obj *Reg2) GetAddress() hal.RegAddress {
	return REG2
}

func (obj *Reg2) GetValue() uint8 {
	return obj.channel
}

func (obj *Reg2) SetValue(value uint8) {
	if value > 80 {
		value = 80
	}
	obj.channel = value
}

// REG3 specification

type enableRSSI uint8

const (
	RSSI_DISABLE enableRSSI = 0x00
	RSSI_ENABLE  enableRSSI = 0x80
)

type transmissionMethod uint8

const (
	TRANSMISSION_TRANSPARENT transmissionMethod = 0x00
	TRANSMISSION_FIXED       transmissionMethod = 0x40
)

type lbt uint8

const (
	LBT_DISABLE lbt = 0x00
	LBT_ENABLE  lbt = 0x08
)

type worCycle uint8

const (
	WOR_500_MS worCycle = iota
	WOR_1000_MS
	WOR_1500_MS
	WOR_2000_MS
	WOR_2500_MS
	WOR_3000_MS
	WOR_3500_MS
	WOR_4000_MS
)

type Reg3 struct {
	enableRSSI         enableRSSI
	transmissionMethod transmissionMethod
	lbtEnable          lbt
	worCycle           worCycle
}

func (obj *Reg3) GetAddress() hal.RegAddress {
	return REG3
}

func (obj *Reg3) GetValue() uint8 {
	return uint8(obj.enableRSSI) | uint8(obj.transmissionMethod) | uint8(obj.lbtEnable) | uint8(obj.worCycle)
}

func (obj *Reg3) SetValue(value uint8) {
	obj.enableRSSI = enableRSSI(value & 0x80)
	obj.transmissionMethod = transmissionMethod(value & 0x40)
	obj.lbtEnable = lbt(value & 0x08)
	obj.worCycle = worCycle(value & 0x07)
}

// CRYPT_H specification

type CryptH struct {
	value uint8
}

func (obj *CryptH) GetAddress() hal.RegAddress {
	return CRYPT_H
}

func (obj *CryptH) GetValue() uint8 {
	return 0
}

func (obj *CryptH) SetValue(value uint8) {
	obj.value = value
}

// CRYPT_L specification

type CryptL struct {
	value uint8
}

func (obj *CryptL) GetAddress() hal.RegAddress {
	return CRYPT_L
}

func (obj *CryptL) GetValue() uint8 {
	return 0
}

func (obj *CryptL) SetValue(value uint8) {
	obj.value = value
}
