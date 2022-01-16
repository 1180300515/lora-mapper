/*
Copyright 2021 The KubeEdge Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import (
	"fmt"
	"sync"

	"github.com/kubeedge/mappers-go/mappers/common"
)

type Lora_tcpclient struct {
	TCPclient *TCPClientHandler
	online    bool
	mu        sync.Mutex
}

type Tcpconfig struct {
	IP             string
	Port           string
	ConcentratorID uint32
	ConverterID    uint32
	SlaveID        int16
}

func NewClient(config Tcpconfig) (client *Lora_tcpclient) {
	addr := config.IP + ":" + config.Port
	fmt.Println("the addr is ", addr)
	client.TCPclient = NewTCPClientHandler(addr)
	client.TCPclient.ConcentratorID = config.ConcentratorID
	client.TCPclient.ConverterID = config.ConverterID
	client.TCPclient.SlaveId = byte(config.SlaveID)
	return
}

func (bc *Lora_tcpclient) Read(data [5]byte) []byte {
	send_package := bc.TCPclient.tcpPackager.Encode(data)
	receivepackage, err := bc.TCPclient.tcpTransporter.Send(send_package)
	if err != nil {
		fmt.Println("error")
	}
	recv := bc.TCPclient.tcpPackager.Decode(receivepackage)
	length := recv.data[2]
	recvdata := recv.data[3 : 2+length]
	if recv.online == 1 {
		bc.online = false
	} else {
		bc.online = true
	}

	return recvdata
}

func (bc *Lora_tcpclient) Write(data [5]byte) []byte {
	send_package := bc.TCPclient.tcpPackager.Encode(data)
	receivepackage, err := bc.TCPclient.tcpTransporter.Send(send_package)
	if err != nil {
		fmt.Println("error")
	}
	recv := bc.TCPclient.tcpPackager.Decode(receivepackage)
	recvdata := recv.data[4:5]
	if recv.online == 1 {
		bc.online = false
	} else {
		bc.online = true
	}

	return recvdata
}

// GetStatus get device status.
// Now we could only get the connection status.
func (bc *Lora_tcpclient) GetStatus() string {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.online {
		return common.DEVSTOK
	}
	return common.DEVSTDISCONN
}
