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

	// create chip handler, read config and update registers model with parameters that are stored on chip
	chip, err := e22.NewChip(hw, messageEvent)
	if err != nil {
		log.Fatal(err)
	}

	hw.SetChipMode(hal.ModeNormal)

	log.Println(chip.GetModuleConfiguration())

	// enable RSSI info in message, otherwise RSSI will be set to 0
	cb := e22.NewConfigUpdateBuilder(chip).RSSIState(e22.RSSI_ENABLE)
	err = cb.WritePermanentConfig()
	if err != nil {
		log.Printf("config write error: %s", err)
	}

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
