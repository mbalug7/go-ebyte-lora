// +build pico
package pico

import (
	"fmt"
	"machine"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
)

const (
	actionPowerReset int32 = iota
	actionRead
	actionWrite
	actionModeSwitch
)

type chipModeLineState struct {
	m0Value int
	m1Value int
}

var chipModes = map[hal.ChipMode]*chipModeLineState{
	hal.ModeNormal:    {m0Value: 0, m1Value: 0},
	hal.ModeWakeUp:    {m0Value: 1, m1Value: 0},
	hal.ModePowerSave: {m0Value: 0, m1Value: 1},
	hal.ModeSleep:     {m0Value: 1, m1Value: 1},
}

type serialPortData struct {
	serialBaud            int
	serialParityBit       hal.Parity
	serialBaudStaged      int
	serialParityBitStaged hal.Parity
}

type HWHandler struct {
	tty            string          // serial port name
	serialPortData *serialPortData // serial port config data
	M0Line         machine.Pin     // M0 GPIO Pin
	M1Line         machine.Pin     // M1 GPIO Pin
	AUXLine        machine.Pin     // AUX GPIO Pin
	serialStream   *machine.UART   // serial port needed communicate with the module
	auxAction      int32           // action that will be executed on rising edge of AUX pin
	muRead         sync.Mutex      // lock reading until previous read is done or timeout
	muBusy         sync.Mutex      // write, and mode change must be locked until previous write or mode switch operation is done
	onMsgCb        hal.OnMessageCb
}

func NewHWHandler(M0Pin machine.Pin, M1Pin machine.Pin, AUXPin machine.Pin, uart *machine.UART) (*HWHandler, error) {

	handler := &HWHandler{
		serialStream: uart,
		serialPortData: &serialPortData{
			serialBaud:            9600,
			serialParityBit:       hal.ParityNone,
			serialBaudStaged:      9600,
			serialParityBitStaged: hal.ParityNone,
		},
		auxAction: actionPowerReset,
	}
	err := uart.Configure(machine.UARTConfig{
		BaudRate: 9600,
		TX:       machine.UART1_TX_PIN,
		RX:       machine.UART1_RX_PIN,
	})
	if err != nil {
		return nil, err
	}

	M0Pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	M1Pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	M0Pin.High()
	M1Pin.High()

	auxIrq := machine.GP11
	auxIrq.Configure(machine.PinConfig{Mode: machine.PinInput})
	auxIrq.SetInterrupt(machine.PinRising, func(p machine.Pin) {
		go handler.onAuxPinRiseEvent()
	})
	time.Sleep(200 * time.Millisecond)
	handler.setAuxAction(actionRead)
	uart.Buffer.Clear()
	return handler, nil
}

func (obj *HWHandler) RegisterOnMessageCb(cb hal.OnMessageCb) error {
	obj.onMsgCb = cb
	return nil
}

func (obj *HWHandler) StageSerialPortConfig(baudRate int, parityBit hal.Parity) {
	obj.serialPortData.serialBaudStaged = baudRate
	obj.serialPortData.serialParityBitStaged = parityBit
}

func (obj *HWHandler) updateSerialConfig(serialPortData *serialPortData) (err error) {

	// ignore updating if current and next config is the same
	if serialPortData.serialBaud == serialPortData.serialBaudStaged &&
		serialPortData.serialParityBit == serialPortData.serialParityBitStaged {
		return nil
	}

	// lock all RW operations until this is finished
	obj.muRead.Lock()
	obj.serialStream.SetBaudRate(uint32(serialPortData.serialBaudStaged))
	serialPortData.serialBaud = serialPortData.serialBaudStaged
	serialPortData.serialParityBit = serialPortData.serialParityBitStaged
	obj.muRead.Unlock()
	return nil
}

func (obj *HWHandler) onAuxPinRiseEvent() {
	// there is a case when we want to write something to serial or switch chip mode, but the module is busy with reading
	// on aux rising edge, module is not busy, and operations that wait can be executed
	currentAction := atomic.LoadInt32(&obj.auxAction)
	if currentAction == actionModeSwitch {
		obj.setAuxAction(actionRead)
		return
	}
	if currentAction == actionWrite {
		obj.setAuxAction(actionRead)
		return
	}
	if currentAction == actionRead {
		data, err := obj.ReadSerial()
		println("DATA READ ON AUX %s", data)
		if obj.onMsgCb != nil && len(data) > 0 {
			obj.onMsgCb(data, err)
		}
		return
	}
}

func (obj *HWHandler) ReadSerial() ([]byte, error) {
	// read all buffered data, before new read can be performed
	obj.muRead.Lock()
	defer obj.muRead.Unlock()

	buf := make([]byte, 512)
	n, err := obj.serialStream.Read(buf)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to receive data: %w", err)
	}
	obj.serialStream.Buffer.Clear()
	if n == 0 {
		return buf, fmt.Errorf("no data")
	}
	return buf[:n], nil
}

func (obj *HWHandler) WriteSerial(msg []byte) error {
	// lock it, another write or mode switch can't happen before this writing finishes
	obj.muBusy.Lock()
	defer obj.muBusy.Unlock()
	obj.setAuxAction(actionWrite)
	obj.serialStream.Buffer.Clear()
	_, err := obj.serialStream.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to send data, err: %w", err)
	}

	// module needs 2ms to switch from busy mode to non busy mode after rising aux edge
	time.Sleep(2 * time.Millisecond)
	return nil
}

func (obj *HWHandler) SetMode(mode hal.ChipMode) error {
	// lock it, another write or mode switch can't happen before this mode switching finishes
	currentMode, err := obj.GetMode()
	if err != nil {
		return err
	}
	if currentMode == mode {
		return nil
	}
	obj.muBusy.Lock()
	defer obj.muBusy.Unlock()
	chipMode, ok := chipModes[mode]
	if !ok {
		return fmt.Errorf("failed to set unsupported chip mode: %d", mode)
	}

	if mode == hal.ModeSleep {
		err := obj.updateSerialConfig(&serialPortData{
			serialBaud:            obj.serialPortData.serialBaud,
			serialParityBit:       obj.serialPortData.serialParityBit,
			serialBaudStaged:      9600,
			serialParityBitStaged: hal.ParityNone,
		})
		if err != nil {
			return fmt.Errorf("failed to setup serial port params for sleep mode, err: %w", err)
		}
	} else {
		err := obj.updateSerialConfig(obj.serialPortData)
		if err != nil {
			return fmt.Errorf("failed to setup serial port params for sleep mode, err: %w", err)
		}
	}

	// set aux action to mode switch
	obj.setAuxAction(actionModeSwitch)

	if chipMode.m0Value == 1 {
		obj.M0Line.High()
	} else {
		obj.M0Line.Low()
	}
	if chipMode.m1Value == 1 {
		obj.M1Line.High()
	} else {
		obj.M1Line.Low()
	}
	// documentation says that the mode switching is not completed on raising edge. It needs 2 ms.
	// waiting 200 just to be sure
	time.Sleep(200 * time.Millisecond)
	return nil
}

func (obj *HWHandler) GetMode() (hal.ChipMode, error) {
	m0ValBool := obj.M0Line.Get()
	m1ValBool := obj.M1Line.Get()

	m0Val := 0
	m1Val := 0
	if m0ValBool {
		m0Val = 1
	}

	if m1ValBool {
		m1Val = 1
	}

	for mode, values := range chipModes {

		if values.m0Value == m0Val && values.m1Value == m1Val {
			return mode, nil
		}
	}
	return 0, fmt.Errorf("chip is in some weird undefined mode. Check connection")
}

func (obj *HWHandler) setAuxAction(action int32) {
	atomic.StoreInt32(&obj.auxAction, action)
	obj.auxAction = action
}
