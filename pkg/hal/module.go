package hal

type Module interface {
	SendMessage(message string) error
	SendFixedMessage(addressHigh byte, addressLow byte, channel byte, message string) error
	GetModuleConfiguration() string
}
