# go-ebyte-lora
EBYTE Lora modules interface library for Linux, Raspberry PI

Experimental phase

WARNING: 
* It was tested on Raspberry Pi 4 Model B, kernel 5.10
* There is possibility that this lib will not work on a lower kernel versions, beacuse it is based on Go gpiod library that needs kernel 5.10+ for HW interrupt handling
* Experimental phase
* E22 EBYTE modules should work with this library
* E32 support will be added
* There is still no documentation, just a several comments

How to connect:
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

```
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mbalug7/go-ebyte-lora/pkg/e22"
	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
)

func messageEvent(msg e22.Message, err error) {
	if err != nil {
		log.Printf("message event error: %s", err)
		return
	}
	log.Printf("NEW MSG DATA: %s", string(msg.Payload))
	log.Printf("NEW MSG RSSI [%d] dBm", msg.RSSI)
}

func main() {
	// create chip hardware handler and put chip in sleep mode
	hw, err := hal.NewCommonHWHandler(23, 24, 25, "/dev/ttyS0", "gpiochip0")
	if err != nil {
		log.Fatal(err)
	}

	// create chip handler, read config and update registers model with parameters that are stored on the chip
	chip, err := e22.NewChip(hw, messageEvent)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(chip.GetModuleConfiguration())
  
  // change chip mode to normal mode and start receiving
	hw.SetChipMode(hal.ModeNormal)

  // config builder is used to update chip with the new parameters 
	// enable RSSI info in message, otherwise RSSI will be set to 0
	cb := e22.NewConfigUpdateBuilder(chip).RSSIState(e22.RSSI_ENABLE)
	err = cb.WritePermanentConfig()
	if err != nil {
		log.Printf("config write error: %s", err)
	}

  // send some test message. Response is received in messageEvent function
	err = chip.SendMessage("ASTATUS")
	if err != nil {
		log.Printf("failed to send data: %s", err)
	}

	signalInterruptChan := make(chan os.Signal, 1)
	signal.Notify(signalInterruptChan, os.Interrupt, syscall.SIGTERM)
	<-signalInterruptChan
	err = hw.Close()
	if err != nil {
		log.Printf("failed to close e32 communication: %s", err)
	}

}
```
