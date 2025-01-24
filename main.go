package main

import (
	"encoding/hex"
	"fmt"

	"github.com/tusmasoma/tcp-ip/pkg/network"
)

func main() {
	network, _ := network.NewTun()
	network.Bind()

	for {
		pkt, _ := network.Read()
		fmt.Print(hex.Dump(pkt.Buf[:pkt.N]))
		network.Write(pkt)
	}
}
