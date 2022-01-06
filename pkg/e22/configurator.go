package e22

type ConfigBuilder struct {
	chip            *Chip
	stagedRegisters [8]Register
}

func NewConfigUpdateBuilder(chip *Chip) *ConfigBuilder {
	return &ConfigBuilder{
		chip:            chip,
		stagedRegisters: chip.registers, // copy current values
	}
}

func (obj *ConfigBuilder) Address(addressHigh uint8, addressLow uint8) *ConfigBuilder {
	addressH := obj.stagedRegisters[ADD_H].(*AddH)
	addressH.address = addressHigh
	addressL := obj.stagedRegisters[ADD_L].(*AddL)
	addressL.address = addressLow
	return obj
}

// REG0 params

func (obj *ConfigBuilder) SerialBaudRate(br baudRate) *ConfigBuilder {
	reg0 := obj.stagedRegisters[REG0].(*Reg0)
	reg0.baudRate = br
	return obj
}

func (obj *ConfigBuilder) SerialParityBit(parityBit parity) *ConfigBuilder {
	reg0 := obj.stagedRegisters[REG0].(*Reg0)
	reg0.parityBit = parityBit
	return obj
}

func (obj *ConfigBuilder) AirDataRate(adRate airDataRate) *ConfigBuilder {
	reg0 := obj.stagedRegisters[REG0].(*Reg0)
	reg0.adRate = adRate
	return obj
}

// REG1 params

func (obj *ConfigBuilder) SubPacketLength(subPacketLength subPacket) *ConfigBuilder {
	reg1 := obj.stagedRegisters[REG1].(*Reg1)
	reg1.subPacket = subPacketLength
	return obj
}

func (obj *ConfigBuilder) RSSIAmbientNoiseState(state rssiAmbientNoise) *ConfigBuilder {
	reg1 := obj.stagedRegisters[REG1].(*Reg1)
	reg1.ambientNoiseRSSI = state
	return obj
}

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

func (obj *ConfigBuilder) RSSIState(state enableRSSI) *ConfigBuilder {
	reg3 := obj.stagedRegisters[REG3].(*Reg3)
	reg3.enableRSSI = state
	return obj
}

func (obj *ConfigBuilder) TransmissionMethod(method transmissionMethod) *ConfigBuilder {
	reg3 := obj.stagedRegisters[REG3].(*Reg3)
	reg3.transmissionMethod = method
	return obj
}

func (obj *ConfigBuilder) LBTState(state lbt) *ConfigBuilder {
	reg3 := obj.stagedRegisters[REG3].(*Reg3)
	reg3.lbtEnable = state
	return obj
}

func (obj *ConfigBuilder) WORCycle(wor worCycle) *ConfigBuilder {
	reg3 := obj.stagedRegisters[REG3].(*Reg3)
	reg3.worCycle = wor
	return obj
}

func (obj *ConfigBuilder) Crypt(cryptHigh uint8, cryptLow uint8) *ConfigBuilder {
	cryptH := obj.stagedRegisters[CRYPT_H].(*CryptH)
	cryptH.value = cryptHigh
	cryptL := obj.stagedRegisters[CRYPT_L].(*CryptL)
	cryptL.value = cryptLow
	return obj
}

func (obj *ConfigBuilder) Finish() error {
	return obj.chip.WriteConfigToChip(obj.stagedRegisters)
}
