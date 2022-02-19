package hal

// RegAddress defines register type
type RegAddress uint8

// ToByte converts register value to a byte type
func (obj RegAddress) ToByte() byte {
	return byte(obj)
}

// Register interface that defines methods to perform RW operations on the physical register
type Register interface {
	GetAddress() RegAddress
	GetValue() uint8
	SetValue(value uint8)
}
