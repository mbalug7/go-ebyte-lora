package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mbalug7/go-ebyte-lora/pkg/e22"
	"github.com/mbalug7/go-ebyte-lora/pkg/hal"
)

func main() {
	e32, err := hal.NewChipHWHandler(23, 24, 25, "/dev/ttyS0", "gpiochip0")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("------------- Entering sleep")
	err = e32.SetChipMode(hal.ModeSleep)
	if err != nil {
		log.Fatal(err)
	}

	err = e32.WriteSerial([]byte{0xC1, 0x00, 0x06})
	if err != nil {
		log.Printf("failed to write bytes %s", err.Error())
	}
	time.Sleep(200 * time.Millisecond)
	data, err := e32.ReadSerial()
	if err != nil {
		log.Fatal(err)
	}

	e22Chip := e22.NewChip(e32)

	e22Chip.SetConfig(data)

	log.Println("-------------- Entering Normal")
	err = e32.SetChipMode(hal.ModeNormal)
	if err != nil {
		log.Fatal(err)
	}
	// log.Println("------------- Entering sleep")
	// err = e32.SetChipMode(hal.ModeSleep)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// err = e32.WriteSerial([]byte{0xC1, 0x04, 0x01})
	// if err != nil {
	// 	log.Printf("failed to write bytes %s", err.Error())
	// }
	// time.Sleep(200 * time.Millisecond)
	// e32.ReadSerial()

	// log.Println("------------ Entering Normal")
	// err = e32.SetChipMode(hal.ModeNormal)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// err = e32.WriteSerial([]byte("ASTATUS"))
	// if err != nil {
	// 	log.Printf("failed to send data %s", err.Error())
	// }

	signalInterruptChan := make(chan os.Signal, 1)
	signal.Notify(signalInterruptChan, os.Interrupt, syscall.SIGTERM)
	<-signalInterruptChan
	err = e32.Close()
	if err != nil {
		log.Printf("failed to close e32 communication: %s", err.Error())
	}

}
