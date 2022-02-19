package hal

// Module interface defines set of methods that are needed to communicate with the module
type Module interface {
	SendMessage(message string) error
	SendFixedMessage(addressHigh byte, addressLow byte, channel byte, message string) error
	GetModuleConfiguration() string
}
