package common

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mazen160/go-random"
	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
	"github.com/tarm/serial"
	"github.com/warthog618/gpiod"
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
	serialParityBit       serial.Parity
	serialBaudStaged      int
	serialParityBitStaged serial.Parity
}

type HWHandler struct {
	tty              string                // serial port name
	serialPortData   *serialPortData       // serial port config data
	M0Line           *gpiod.Line           // M0 GPIO Pin
	M1Line           *gpiod.Line           // M1 GPIO Pin
	AUXLine          *gpiod.Line           // AUX GPIO Pin
	serialStream     *serial.Port          // serial port needed communicate with the module
	auxAction        int32                 // action that will be executed on rising edge of AUX pin
	auxBusyWaitGroup map[string]chan error // holds channels that wait for raising AUX edge
	writeDone        chan bool             // channel used to notify writer that writing is done on rising AUX edge
	modeSwitchDone   chan bool             // channel used to notify mode switcher that switching is done on rising AUX edge
	muAuxDone        sync.Mutex            // map protection mutex
	muRead           sync.Mutex            // lock reading until previous read is done or timeout
	muBusy           sync.Mutex            // write, and mode change must be locked until previous write or mode switch operation is done
	onMsgCb          hal.OnMessageCb
}

func NewHWHandler(M0Pin int, M1Pin int, AUXPin int, ttyName string, gpioChip string) (*HWHandler, error) {
	handler := &HWHandler{
		tty: ttyName,
		serialPortData: &serialPortData{
			serialBaud:            9600,
			serialParityBit:       serial.ParityNone,
			serialBaudStaged:      9600,
			serialParityBitStaged: serial.ParityNone,
		},
		auxBusyWaitGroup: make(map[string]chan error),
		writeDone:        make(chan bool, 1),
		modeSwitchDone:   make(chan bool, 1),
		auxAction:        actionPowerReset,
	}
	config := &serial.Config{
		Name:        ttyName,
		Baud:        handler.serialPortData.serialBaud,
		Size:        8,
		ReadTimeout: 2 * time.Second,
	}
	var err error
	c, err := gpiod.NewChip(gpioChip, gpiod.WithConsumer("ebyte-module"))
	if err != nil {
		return nil, fmt.Errorf("failed to create GPIO chip: %w", err)
	}

	handler.AUXLine, err = c.RequestLine(AUXPin, gpiod.WithEventHandler(handler.onAuxPinRiseEvent), gpiod.WithRisingEdge)
	if err != nil {
		return nil, fmt.Errorf("failed to request AUX GPIO line: %w", err)
	}

	handler.M0Line, err = c.RequestLine(M0Pin, gpiod.AsOutput(1))
	if err != nil {
		return nil, fmt.Errorf("failed to request M0 GPIO line: %w", err)
	}

	handler.M1Line, err = c.RequestLine(M1Pin, gpiod.AsOutput(1))
	if err != nil {
		return nil, fmt.Errorf("failed to request M1 GPIO line: %w", err)
	}
	handler.serialStream, err = serial.OpenPort(config)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port, err: %w", err)
	}
	time.Sleep(200 * time.Millisecond)
	handler.setAuxAction(actionRead)
	return handler, nil
}

func (obj *HWHandler) Close() (err error) {
	err = obj.M0Line.Close()
	if err != nil {
		return fmt.Errorf("failed to close M0 line: %w", err)
	}
	err = obj.M1Line.Close()
	if err != nil {
		return fmt.Errorf("failed to close M1 line: %w", err)
	}
	err = obj.AUXLine.Close()
	if err != nil {
		return fmt.Errorf("failed to close AUX line: %w", err)
	}

	err = obj.serialStream.Close()
	if err != nil {
		return fmt.Errorf("failed to close serial stream: %w", err)
	}
	return nil
}

func (obj *HWHandler) RegisterOnMessageCb(cb hal.OnMessageCb) error {
	if obj.onMsgCb != nil {
		return fmt.Errorf("on message callback already registered")
	}
	obj.onMsgCb = cb
	return nil
}

func (obj *HWHandler) StageSerialPortConfig(baudRate int, parityBit serial.Parity) {
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
	defer obj.muRead.Unlock()

	if obj.serialStream != nil {
		err := obj.serialStream.Flush()
		if err != nil {
			return fmt.Errorf("failed to flush serial stream: %w", err)
		}
		err = obj.serialStream.Close()
		if err != nil {
			return fmt.Errorf("failed to close serial stream: %w", err)
		}
	}

	config := &serial.Config{
		Name:        obj.tty,
		Baud:        serialPortData.serialBaudStaged,
		Size:        8,
		ReadTimeout: 2 * time.Second,
		Parity:      serialPortData.serialParityBitStaged,
	}
	obj.serialStream, err = serial.OpenPort(config)
	if err != nil {
		return fmt.Errorf("failed to open serial port, err: %w", err)
	}
	serialPortData.serialBaud = serialPortData.serialBaudStaged
	serialPortData.serialParityBit = serialPortData.serialParityBitStaged
	return nil
}

func (obj *HWHandler) onAuxPinRiseEvent(evt gpiod.LineEvent) {
	// there is a case when we want to write something to serial or switch chip mode, but the module is busy with reading
	// on aux rising edge, module is not busy, and operations that wait can be executed
	defer obj.auxDoneNotifyReceivers()

	currentAction := atomic.LoadInt32(&obj.auxAction)
	if currentAction == actionModeSwitch {
		obj.setAuxAction(actionRead)
		obj.modeSwitchDone <- true
		return
	}
	if currentAction == actionWrite {
		obj.setAuxAction(actionRead)
		obj.writeDone <- true
		return
	}
	if currentAction == actionRead {
		data, err := obj.ReadSerial()
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
	return buf[:n], nil
}

func (obj *HWHandler) WriteSerial(msg []byte) error {
	// lock it, another write or mode switch can't happen before this writing finishes
	obj.muBusy.Lock()
	defer obj.muBusy.Unlock()

	// check if module is busy, wait for previous action to finish
	err := obj.registerAndWaitAUXDone()
	if err != nil {
		return fmt.Errorf("failed to check AUX pin input state: %w", err)
	}
	obj.setAuxAction(actionWrite)

	_, err = obj.serialStream.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to send data, err: %w", err)
	}

	select {
	case <-time.After(2 * time.Second):
		return fmt.Errorf("failed to send data, timeout ocurred")
	case <-obj.writeDone:
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
			serialParityBitStaged: serial.ParityNone,
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
	// check if module is busy, wait for previous action to finish
	err = obj.registerAndWaitAUXDone()
	if err != nil {
		return fmt.Errorf("failed to check AUX pin input state: %w", err)
	}

	// set aux action to mode switch
	obj.setAuxAction(actionModeSwitch)

	err = obj.M0Line.SetValue(chipMode.m0Value)
	if err != nil {
		return fmt.Errorf("failed to set mode [%d] on M0 line, err: %w", mode, err)
	}

	err = obj.M1Line.SetValue(chipMode.m1Value)
	if err != nil {
		return fmt.Errorf("failed to set mode [%d] on M1 line, err %w", mode, err)
	}

	select {
	case <-time.After(2 * time.Second):
		return fmt.Errorf("failed to switch chip mode, timeout ocurred")
	case <-obj.modeSwitchDone:
	}
	// documentation says that the mode switching is not completed on raising edge. It needs 2 ms.
	// waiting 200 just to be sure
	time.Sleep(200 * time.Millisecond)
	return nil
}

func (obj *HWHandler) auxDoneNotifyReceivers() {
	obj.muAuxDone.Lock()
	defer obj.muAuxDone.Unlock()
	for id, ch := range obj.auxBusyWaitGroup {
		ch <- nil
		close(ch)
		delete(obj.auxBusyWaitGroup, id)
	}

}

func (obj *HWHandler) registerAndWaitAUXDone() error {
	val, err := obj.AUXLine.Value()
	if err != nil {
		return err
	}
	if val == 1 {
		return nil
	}

	ch := make(chan error)
	id, err := random.String(16)
	if err != nil {
		return fmt.Errorf("failed to generate random id: %w", err)
	}
	obj.muAuxDone.Lock()
	obj.auxBusyWaitGroup[id] = ch
	obj.muAuxDone.Unlock()
	select {
	case <-time.After(2 * time.Second):
		return fmt.Errorf("aux free checking timeouted")
	case <-ch:
		return nil
	}

}

func (obj *HWHandler) GetMode() (hal.ChipMode, error) {
	m0Val, err := obj.M0Line.Value()
	if err != nil {
		return 0, fmt.Errorf("failed to get M0 line value, err: %w", err)
	}
	m1Val, err := obj.M1Line.Value()
	if err != nil {
		return 0, fmt.Errorf("failed to get M0 line value, err: %w", err)
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
