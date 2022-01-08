package hal

import (
	"fmt"
	"sync"
	"time"

	"github.com/mazen160/go-random"
	"github.com/tarm/serial"
	"github.com/warthog618/gpiod"
)

type auxAction int

const (
	actionPowerReset auxAction = iota
	actionRead
	actionWrite
	actionModeSwitch
)

type chipModeLineState struct {
	m0Value int
	m1Value int
}

var chipModes = map[ChipMode]*chipModeLineState{
	ModeNormal:    {m0Value: 0, m1Value: 0},
	ModeWakeUp:    {m0Value: 1, m1Value: 0},
	ModePowerSave: {m0Value: 0, m1Value: 1},
	ModeSleep:     {m0Value: 1, m1Value: 1},
}

type serialPortData struct {
	serialBaud            int
	serialParityBit       serial.Parity
	serialBaudStaged      int
	serialParityBitStaged serial.Parity
}

type CommonHWHandler struct {
	tty              string
	serialPortData   *serialPortData
	M0Line           *gpiod.Line
	M1Line           *gpiod.Line
	AUXLine          *gpiod.Line
	serialStream     *serial.Port
	auxAction        auxAction
	muAuxBusyWgMap   sync.Mutex            // map protection mutex
	auxBusyWaitGroup map[string]chan error // holds channels that wait for raising AUX edge
	writeDone        chan bool             // channel used to notify writer that writing is done on rising AUX edge
	modeSwitchDone   chan bool             // channel used to notify mode switcher that switching is done on rising AUX edge
	muRead           sync.Mutex            // lock reading until previous read is done or timeout
	muBusy           sync.Mutex            // write, and mode change must be locked until previous write or mode switch operation is done
	onMsgCb          OnMessageCb
}

func NewCommonHWHandler(M0Pin int, M1Pin int, AUXPin int, ttyName string, gpioChip string) (*CommonHWHandler, error) {
	e32 := &CommonHWHandler{
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
		Baud:        e32.serialPortData.serialBaud,
		Size:        8,
		ReadTimeout: 2 * time.Second,
	}
	var err error
	c, err := gpiod.NewChip(gpioChip, gpiod.WithConsumer("glora32"))
	if err != nil {
		return nil, fmt.Errorf("failed to create GPIO chip: %w", err)
	}

	e32.AUXLine, err = c.RequestLine(AUXPin, gpiod.WithEventHandler(e32.onAuxPinRiseEvent), gpiod.WithRisingEdge)
	if err != nil {
		return nil, fmt.Errorf("failed to request AUX GPIO line: %w", err)
	}

	e32.M0Line, err = c.RequestLine(M0Pin, gpiod.AsOutput(1))
	if err != nil {
		return nil, fmt.Errorf("failed to request M0 GPIO line: %w", err)
	}

	e32.M1Line, err = c.RequestLine(M1Pin, gpiod.AsOutput(1))
	if err != nil {
		return nil, fmt.Errorf("failed to request M1 GPIO line: %w", err)
	}
	e32.serialStream, err = serial.OpenPort(config)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port, err: %w", err)
	}
	time.Sleep(200 * time.Millisecond)
	e32.setAuxAction(actionRead)
	return e32, nil
}

func (obj *CommonHWHandler) Close() (err error) {
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

func (obj *CommonHWHandler) RegisterOnMessageCb(cb OnMessageCb) error {
	if obj.onMsgCb != nil {
		return fmt.Errorf("on message callback already registered")
	}
	obj.onMsgCb = cb
	return nil
}

func (obj *CommonHWHandler) StageSerialPortConfig(baudRate int, parityBit serial.Parity) {
	obj.serialPortData.serialBaudStaged = baudRate
	obj.serialPortData.serialParityBitStaged = parityBit
}

func (obj *CommonHWHandler) updateSerialConfig(serialPortData *serialPortData) (err error) {

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

func (obj *CommonHWHandler) onAuxPinRiseEvent(evt gpiod.LineEvent) {
	// there is a case when we want to write something to serial or switch chip mode, but the module is busy with reading
	// on aux rising edge, module is not busy, and operations that wait can be executed
	defer obj.auxDoneNotifyReceivers()

	if obj.auxAction == actionModeSwitch {
		obj.setAuxAction(actionRead)
		obj.modeSwitchDone <- true
		return
	}
	if obj.auxAction == actionWrite {
		obj.setAuxAction(actionRead)
		obj.writeDone <- true
		return
	}
	if obj.auxAction == actionRead {
		data, err := obj.ReadSerial()
		if obj.onMsgCb != nil {
			obj.onMsgCb(data, err)
		}
		return
	}
}

func (obj *CommonHWHandler) ReadSerial() ([]byte, error) {
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

func (obj *CommonHWHandler) WriteSerial(msg []byte) error {
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

func (obj *CommonHWHandler) SetChipMode(mode ChipMode) error {
	// lock it, another write or mode switch can't happen before this mode switching finishes
	currentMode, err := obj.GetChipMode()
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

	if mode == ModeSleep {
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

func (obj *CommonHWHandler) auxDoneNotifyReceivers() {
	obj.muAuxBusyWgMap.Lock()
	defer obj.muAuxBusyWgMap.Unlock()
	for id, ch := range obj.auxBusyWaitGroup {
		ch <- nil
		close(ch)
		delete(obj.auxBusyWaitGroup, id)
	}

}

func (obj *CommonHWHandler) registerAndWaitAUXDone() error {
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
	obj.muAuxBusyWgMap.Lock()
	obj.auxBusyWaitGroup[id] = ch
	obj.muAuxBusyWgMap.Unlock()
	select {
	case <-time.After(2 * time.Second):
		return fmt.Errorf("aux free checking timeouted")
	case <-ch:
		return nil
	}

}

func (obj *CommonHWHandler) GetChipMode() (ChipMode, error) {
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

func (obj *CommonHWHandler) setAuxAction(action auxAction) {
	obj.auxAction = action
}
