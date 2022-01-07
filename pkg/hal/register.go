package hal

type RegAddress uint8

type Register interface {
	GetAddress() RegAddress
	GetValue() uint8
	SetValue(value uint8)
}
