package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mbalug7/go-ebyte-lora/pkg/e22"
	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
	"github.com/mbalug7/go-ebyte-lora/pkg/rpi"
)

// messageEvent callback method that is called when new message is received
func messageEvent(msg e22.Message, err error) {
	if err != nil {
		log.Printf("message event error: %s", err)
		return
	}
	log.Printf("DATA: %s", string(msg.Payload))
	log.Printf("RSSI [-%d dBm]", msg.RSSI)
}

func main() {
	// create common hw hardware handler, this HW handler works with E22 and E32 modules
	// M0 -> GPIO 23
	// M1 -> GPIO 24
	// AUX -> GPIO 25
	// /dev/ttyS0 -> RPi 4 serial
	// gpiochip0 -> RPi4 GPIO chip name, 5.5+ Linux kernel needed
	hw, err := rpi.NewHWHandler(23, 24, 25, "/dev/ttyS0", "gpiochip0")
	if err != nil {
		log.Fatal(err)
	}

	// create E22 module handler
	// when creating a new module, config parameters from E22 module are read, and synchronized with the local registers model
	module, err := e22.NewModule(hw, messageEvent)
	if err != nil {
		log.Fatal(err)
	}

	// E22 operating mode must be explicitly defined after initialization
	hw.SetMode(hal.ModeNormal)

	// print current configuration
	log.Println(module.GetModuleConfiguration())

	// ConfigBuilder is used to create a new module config
	// enable RSSI info in received messages
	// in this example only RSSIState flag is updated, and nothing else. All other registers values ​​are preserved.
	// cb := e22.NewConfigBuilder(module).RSSIState(e22.RSSI_ENABLE)
	// err = cb.WritePermanentConfig() // update registers on the module with the new data
	// if err != nil {
	// 	// log write error
	// 	log.Printf("config write error: %s", err)
	// } else {
	// }
	log.Println(module.GetModuleConfiguration())

	cb := e22.NewConfigBuilder(module).RSSIState(e22.RSSI_DISABLE).Address(0, 3).Channel(23).AirDataRate(e22.ADR_2400).TransmissionMethod(e22.TRANSMISSION_FIXED)
	err = cb.WritePermanentConfig() // update registers on the module with the new data
	if err != nil {
		// log write error
		log.Printf("config write error: %s", err)
	} else {
		log.Println(module.GetModuleConfiguration())
	}

	// send some message, and expect response in `messageEvent` func`
	err = module.SendMessage("ASTATUS")
	if err != nil {
		log.Printf("failed to send data: %s", err)
	}

	// wait for keyboard signal interrupt
	signalInterruptChan := make(chan os.Signal, 1)
	signal.Notify(signalInterruptChan, os.Interrupt, syscall.SIGTERM)
	<-signalInterruptChan
	err = hw.Close()
	if err != nil {
		log.Printf("failed to close communication with the module: %s", err)
	}

}
