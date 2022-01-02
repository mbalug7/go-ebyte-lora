package hal

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
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

type chipMode int

const (
	ModeNormal chipMode = iota
	ModeWakeUp
	ModePowerSave
	ModeSleep
)

type chipModeLineState struct {
	m0Value int
	m1Value int
}

var chipModes = map[chipMode]*chipModeLineState{
	ModeNormal:    {m0Value: 0, m1Value: 0},
	ModeWakeUp:    {m0Value: 1, m1Value: 0},
	ModePowerSave: {m0Value: 0, m1Value: 1},
	ModeSleep:     {m0Value: 1, m1Value: 1},
}

type ChipHWHandler struct {
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
}

func NewChipHWHandler(M0Pin int, M1Pin int, AUXPin int, ttyName string, gpioChip string) (*ChipHWHandler, error) {
	e32 := &ChipHWHandler{
		auxBusyWaitGroup: make(map[string]chan error),
		writeDone:        make(chan bool, 1),
		modeSwitchDone:   make(chan bool, 1),
		auxAction:        actionPowerReset,
	}
	config := &serial.Config{
		Name:        ttyName,
		Baud:        9600,
		Size:        8,
		ReadTimeout: 2 * time.Second,
	}
	var err error
	c, err := gpiod.NewChip(gpioChip, gpiod.WithConsumer("glora32"))
	if err != nil {
		return nil, fmt.Errorf("failed to create GPIO chip: %s", err.Error())
	}

	e32.AUXLine, err = c.RequestLine(AUXPin, gpiod.WithEventHandler(e32.onAuxPinRiseEvent), gpiod.WithRisingEdge)
	if err != nil {
		return nil, fmt.Errorf("failed to request AUX GPIO line %s", err.Error())
	}

	e32.M0Line, err = c.RequestLine(M0Pin, gpiod.AsOutput(0))
	if err != nil {
		return nil, fmt.Errorf("failed to request M0 GPIO line %s", err.Error())
	}

	e32.M1Line, err = c.RequestLine(M1Pin, gpiod.AsOutput(0))
	if err != nil {
		return nil, fmt.Errorf("failed to request M1 GPIO line %s", err.Error())
	}
	e32.serialStream, err = serial.OpenPort(config)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port, err: %s", err.Error())
	}
	time.Sleep(200 * time.Millisecond)
	e32.setAuxAction(actionRead)
	return e32, nil
}

func (obj *ChipHWHandler) Close() (err error) {
	err = obj.M0Line.Close()
	if err != nil {
		return fmt.Errorf("failed to close M0 line %s", err)
	}
	err = obj.M1Line.Close()
	if err != nil {
		return fmt.Errorf("failed to close M1 line %s", err)
	}
	err = obj.AUXLine.Close()
	if err != nil {
		return fmt.Errorf("failed to close AUX line %s", err)
	}

	err = obj.serialStream.Close()
	if err != nil {
		return fmt.Errorf("failed to close serial stream %s", err)
	}
	return nil
}

func (obj *ChipHWHandler) onAuxPinRiseEvent(evt gpiod.LineEvent) {
	log.Printf("AUX PIN UPDATE %+v", evt)
	// there is a case when we want to write something to serial or switch chip mode, but the module is busy with reading
	// on aux rising edge, module is not busy, and operations that wait can be executed
	defer obj.auxDoneNotifyReceivers()

	if obj.auxAction == actionModeSwitch {
		log.Println("mode switch done")
		obj.setAuxAction(actionRead)
		obj.modeSwitchDone <- true
		return
	}
	if obj.auxAction == actionWrite {
		log.Println("action write done")
		obj.setAuxAction(actionRead)
		obj.writeDone <- true
		return
	}
	if obj.auxAction == actionRead {
		obj.ReadSerial()
		return
	}
}

func (obj *ChipHWHandler) ReadSerial() (string, error) {
	log.Printf("READ STARTED")

	// read all buffered data, before new read can be performed
	obj.muRead.Lock()
	defer obj.muRead.Unlock()

	buf := make([]byte, 512)
	n, err := obj.serialStream.Read(buf)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		log.Printf("failed to receive data %s", err.Error())
		return "", fmt.Errorf("failed to receive data %s", err.Error())
	}
	log.Printf("Data Received bytes: %s \n", hex.EncodeToString(buf[:n]))
	log.Printf("Data Received string: %s\n", string(buf[:n]))
	return string(buf[:n]), nil
}

func (obj *ChipHWHandler) WriteSerial(msg []byte) error {
	// lock it, another write or mode switch can't happen before this writing finishes
	obj.muBusy.Lock()
	defer obj.muBusy.Unlock()

	// check if module is busy, wait for previous action to finish
	err := obj.registerAndWaitAUXDone()
	if err != nil {
		return fmt.Errorf("failed to check AUX pin input state: %s", err.Error())
	}
	log.Printf("writing string %v", msg)

	obj.setAuxAction(actionWrite)

	_, err = obj.serialStream.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to send data, err: %s", err.Error())
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

func (obj *ChipHWHandler) SetChipMode(mode chipMode) error {
	// lock it, another write or mode switch can't happen before this mode switching finishes
	obj.muBusy.Lock()
	defer obj.muBusy.Unlock()
	chipMode, ok := chipModes[mode]
	if !ok {
		return fmt.Errorf("failed to set unsupported chip mode: %d", mode)
	}

	// check if module is busy, wait for previous action to finish
	err := obj.registerAndWaitAUXDone()
	if err != nil {
		return fmt.Errorf("failed to check AUX pin input state: %s", err.Error())
	}

	// set aux action to mode switch
	obj.setAuxAction(actionModeSwitch)

	err = obj.M0Line.SetValue(chipMode.m0Value)
	if err != nil {
		return fmt.Errorf("failed to set mode [%d] on M0 line, err: %s", mode, err.Error())
	}

	err = obj.M1Line.SetValue(chipMode.m1Value)
	if err != nil {
		return fmt.Errorf("failed to set mode [%d] on M1 line, err %s", mode, err.Error())
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

func (obj *ChipHWHandler) auxDoneNotifyReceivers() {
	obj.muAuxBusyWgMap.Lock()
	defer obj.muAuxBusyWgMap.Unlock()
	for id, ch := range obj.auxBusyWaitGroup {
		log.Printf("notifying %s", id)
		ch <- nil
		close(ch)
		delete(obj.auxBusyWaitGroup, id)
	}

}

func (obj *ChipHWHandler) registerAndWaitAUXDone() error {
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
		return fmt.Errorf("failed to generate random id: %s", err.Error())
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

func (obj *ChipHWHandler) setAuxAction(action auxAction) {
	obj.auxAction = action
}