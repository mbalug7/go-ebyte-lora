package hal

type RegAddress uint8

func (obj RegAddress) ToByte() byte {
	return byte(obj)
}

type Register interface {
	GetAddress() RegAddress
	GetValue() uint8
	SetValue(value uint8)
}
