# go-ebyte-lora
EBYTE interface library for Linux, Raspberry PI, written in Go.

Alpha

WARNING:
* Tested on Raspberry Pi 4 Model B, kernel 5.5+
* There is possibility that this lib will not work on a lower kernel versions, because it is based on Go gpiod library that needs kernel 5.5+ for proper HW interrupt handling
* Lib is stil in experimental phase. There is no documentation and tests, for now. 
* E22 EBYTE modules should be fully supported
* E32 support will be added

How to connect E22 module to RPi:
- `RX -> RPI TX`
- `TX -> RPI RX`
- `AUX -> GPIO 25`
- `M0 -> GPIO 23`
- `M1 -> GPIO 24`
- `VCC -> RPI 5V`
- `GND -> RPI GND`


E22 Example:

EBYTE E22 chip family should be fully supported by this library.

* Library is tested on E220-900T30D module, and the module documentation can be found here:
  * https://www.ebyte.com/en/downpdf.aspx?id=1214

```Go
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mbalug7/go-ebyte-lora/pkg/common"
	"github.com/mbalug7/go-ebyte-lora/pkg/e22"
	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
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
	hw, err := common.NewHWHandler(23, 24, 25, "/dev/ttyS0", "gpiochip0")
	if err != nil {
		log.Fatal(err)
	}

	// create E22 module handler
	// when creating a new module, current configuration that is stored on E22 module is fetched, and synchronized with the local registers model
	module, err := e22.NewModule(hw, messageEvent)
	if err != nil {
		log.Fatal(err)
	}

	hw.SetMode(hal.ModeNormal) // E22 operating mode must be explicitly defined after initialization

	// print current configuration
	log.Println(module.GetModuleConfiguration())

	// ConfigBuilder is used to create a new module config
	// enable RSSI info in received messages
	// in this example only RSSIState flag is updated, and nothing else. All other registers values are preserved.
	cb := e22.NewConfigBuilder(module).RSSIState(e22.RSSI_ENABLE)
	err = cb.WritePermanentConfig() // update registers on the module with the new data
	if err != nil {
		// log write error
		log.Printf("config write error: %s", err)
	} else {
		log.Println(module.GetModuleConfiguration())
	}

	// send some message to Lora EBYTE receiver, and expect response in `messageEvent`  callback function
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
```
