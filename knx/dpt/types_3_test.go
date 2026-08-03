package dpt

import (
	"github.com/vapourismo/knx-go/knx"
	"github.com/vapourismo/knx-go/knx/cemi"
	"log"
	"testing"
)

func TestName(t *testing.T) {
	client, err := knx.NewGroupTunnel("192.168.123.200:3671", knx.DefaultTunnelConfig)
	if err != nil {
		log.Fatal(err)
	}

	// Close upon exiting. Even if the gateway closes the connection, we still have to clean up.
	defer client.Close()

	// Send 20.5°C to group 1/2/3.
	err = client.Send(knx.GroupEvent{
		Command:     knx.GroupWrite,
		Destination: cemi.NewGroupAddr3(1, 6, 0),
		Data:        DPT_3007(12).Pack(),
	})
	if err != nil {
		log.Fatal(err)
	}

}
