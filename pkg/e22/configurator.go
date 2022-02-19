package e22

// ConfigBuilder object that is used to build eByte E22 config
// it is possible to reconfigure only one  parameter
type ConfigBuilder struct {
	chip            *Module
	stagedRegisters registersCollection
}

// NewConfigBuilder constructs ConfigBuilder
func NewConfigBuilder(chip *Module) *ConfigBuilder {
	return &ConfigBuilder{
		chip:            chip,
		stagedRegisters: chip.registers.Copy(), // copy current values
	}
}

// Address set module address
func (obj *ConfigBuilder) Address(addressHigh uint8, addressLow uint8) *ConfigBuilder {
	addressH := obj.stagedRegisters[ADD_H].(*AddH)
	addressH.address = addressHigh
	addressL := obj.stagedRegisters[ADD_L].(*AddL)
	addressL.address = addressLow
	return obj
}

// REG0 params
// SerialBaudRate set module baud rate
func (obj *ConfigBuilder) SerialBaudRate(br baudRate) *ConfigBuilder {
	reg0 := obj.stagedRegisters[REG0].(*Reg0)
	reg0.baudRate = br
	return obj
}

// SerialParityBit set module serial parity bit
func (obj *ConfigBuilder) SerialParityBit(parityBit parity) *ConfigBuilder {
	reg0 := obj.stagedRegisters[REG0].(*Reg0)
	reg0.parityBit = parityBit
	return obj
}

// AirDataRate module data rate
func (obj *ConfigBuilder) AirDataRate(adRate airDataRate) *ConfigBuilder {
	reg0 := obj.stagedRegisters[REG0].(*Reg0)
	reg0.adRate = adRate
	return obj
}

// REG1 params
// SubPacketLength set module data packet length
func (obj *ConfigBuilder) SubPacketLength(subPacketLength subPacket) *ConfigBuilder {
	reg1 := obj.stagedRegisters[REG1].(*Reg1)
	reg1.subPacket = subPacketLength
	return obj
}

// RSSIAmbientNoiseState set rssi ambient noise state
func (obj *ConfigBuilder) RSSIAmbientNoiseState(state rssiAmbientNoise) *ConfigBuilder {
	reg1 := obj.stagedRegisters[REG1].(*Reg1)
	reg1.ambientNoiseRSSI = state
	return obj
}

// TransmittingPower set transmitting power
func (obj *ConfigBuilder) TransmittingPower(power transmittingPower) *ConfigBuilder {
	reg1 := obj.stagedRegisters[REG1].(*Reg1)
	reg1.transmittingPower = power
	return obj
}

// REG2 params

// Channel sets chip channel, range 0-80, Actual frequency = 850.125 + CH *1M
func (obj *ConfigBuilder) Channel(channel uint8) *ConfigBuilder {
	reg2 := obj.stagedRegisters[REG2].(*Reg2)
	// set value limits max value to 80 -> chip supports 80 channels
	reg2.SetValue(channel)
	return obj
}

// REG 3
// RSSIState enable rssi value in received message
func (obj *ConfigBuilder) RSSIState(state enableRSSI) *ConfigBuilder {
	reg3 := obj.stagedRegisters[REG3].(*Reg3)
	reg3.enableRSSI = state
	return obj
}

// TransmissionMethod select transparent or fixed method
func (obj *ConfigBuilder) TransmissionMethod(method transmissionMethod) *ConfigBuilder {
	reg3 := obj.stagedRegisters[REG3].(*Reg3)
	reg3.transmissionMethod = method
	return obj
}

// LBTState set lbt state
func (obj *ConfigBuilder) LBTState(state lbt) *ConfigBuilder {
	reg3 := obj.stagedRegisters[REG3].(*Reg3)
	reg3.lbtEnable = state
	return obj
}

// set wake on receive cycle
func (obj *ConfigBuilder) WORCycle(wor worCycle) *ConfigBuilder {
	reg3 := obj.stagedRegisters[REG3].(*Reg3)
	reg3.worCycle = wor
	return obj
}

// Crypt set encryption key that is not readable, make sure that other side uses the same key
func (obj *ConfigBuilder) Crypt(cryptHigh uint8, cryptLow uint8) *ConfigBuilder {
	cryptH := obj.stagedRegisters[CRYPT_H].(*CryptH)
	cryptH.value = cryptHigh
	cryptL := obj.stagedRegisters[CRYPT_L].(*CryptL)
	cryptL.value = cryptLow
	return obj
}

// WritePermanentConfig writes new config to the chip
func (obj *ConfigBuilder) WritePermanentConfig() error {
	return obj.chip.WriteConfigToChip(false, obj.stagedRegisters)
}

// WriteTemporaryConfig writes new config to the chip but, on chip reboot config is lost
func (obj *ConfigBuilder) WriteTemporaryConfig() error {
	return obj.chip.WriteConfigToChip(true, obj.stagedRegisters)
}
