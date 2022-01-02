# go-ebyte-lora
EBYTE Lora modules interface library for Linux, Raspberry PI

Super alpha version

WARNING: It was tested on Raspberry Pi 4, Serial Read, Write, and mode switching on module were working fine. It was tested on kernel 5.10+. It is possible that this lib will not work on a lower kernel versions, beacuse it is based on Go gpiod library that needs kernel 5.10+ for HW interrupt handling.

How to connect:
RX -> RPI TX
TX -> RPI RX
AUX -> GPIO 25
M0 -> GPIO 23
M1 -> GPIO 24
VCC -> RPI 5V
GND -> RPI GND
