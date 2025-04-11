package main

import (
	"qemu-socket-test/vmcore"
	"time"
)

func main() {
	vm_core := vmcore.GetInstance()

	vm_core.ListVMs()

	for {
		time.Sleep(1 * time.Second)
	}
}
