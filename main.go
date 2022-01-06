package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mbalug7/go-ebyte-lora/pkg/e22"
	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
)

func main() {
	// create chip hardware handler and put chip in sleep mode
	hw, err := hal.NewChipHWHandler(23, 24, 25, "/dev/ttyS0", "gpiochip0")
	if err != nil {
		log.Fatal(err)
	}

	// create chip handler, read config and update registers model with parameters that are stored on chip
	chip, err := e22.NewChip(hw)
	if err != nil {
		log.Fatal(err)
	}

	// create config builder, set baud rate and the next chip mode
	// when writing config to chip, chip must be in sleep mode, and after that chip mode will be set to ModeNormal if NextMode is not provided
	cb := e22.NewConfigUpdateBuilder(chip).SerialBaudRate(e22.BAUD_9600).NextMode(hal.ModeNormal)
	err = cb.WritePermanentConfig()
	if err != nil {
		log.Fatal(err)
	}

	err = hw.WriteSerial([]byte("ASTATUS"))
	if err != nil {
		log.Printf("failed to send data %s", err.Error())
	}

	signalInterruptChan := make(chan os.Signal, 1)
	signal.Notify(signalInterruptChan, os.Interrupt, syscall.SIGTERM)
	<-signalInterruptChan
	err = hw.Close()
	if err != nil {
		log.Printf("failed to close e32 communication: %s", err.Error())
	}

}
