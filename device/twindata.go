/*
Copyright 2020 The KubeEdge Authors.

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

package device

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/kubeedge/mappers-go/mappers/common"
	"github.com/kubeedge/mappers-go/mappers/lora/configmap"
	"github.com/kubeedge/mappers-go/mappers/lora/driver"
	"github.com/kubeedge/mappers-go/mappers/lora/globals"
)

// TwinData is the timer structure for getting twin/data.
type TwinData struct {
	Client        *driver.Lora_tcpclient
	Name          []string
	Type          []string
	VisitorConfig []configmap.LoraVisitorConfig
	//	Results       []byte
	Topic string
}

func SwitchRegister(value []byte) []byte {
	for i := 0; i < len(value)/2; i = i + 2 {
		j := len(value) - i - 2
		value[i], value[j] = value[j], value[i]
		value[i+1], value[j+1] = value[j+1], value[i+1]
	}
	return value
}

func SwitchByte(value []byte) []byte {
	if len(value) < 2 {
		return value
	}
	for i := 0; i < len(value); i = i + 2 {
		value[i], value[i+1] = value[i+1], value[i]
	}
	return value
}

func TransferData(isRegisterSwap bool, isSwap bool,
	dataType string, scale float64,
	value []byte) (string, error) {
	// accord IsSwap/IsRegisterSwap to transfer byte array
	if isRegisterSwap {
		SwitchRegister(value)
	}
	if isSwap {
		SwitchByte(value)
	}

	switch dataType {
	case "int":
		switch len(value) {
		case 1:
			data := float64(value[0]) * scale
			sData := strconv.FormatInt(int64(data), 10)
			return sData, nil
		case 2:
			data := float64(binary.BigEndian.Uint16(value)) * scale
			sData := strconv.FormatInt(int64(data), 10)
			return sData, nil
		case 4:
			data := float64(binary.BigEndian.Uint32(value)) * scale
			sData := strconv.FormatInt(int64(data), 10)
			return sData, nil
		case 8:
			data := float64(binary.BigEndian.Uint64(value)) * scale
			sData := strconv.FormatInt(int64(data), 10)
			return sData, nil
		default:
			return "", errors.New("BytesToInt bytes length is invalid")
		}
	case "double":
		if len(value) != 8 {
			return "", errors.New("BytesToDouble bytes length is invalid")
		}
		bits := binary.BigEndian.Uint64(value)
		data := math.Float64frombits(bits) * scale
		sData := strconv.FormatFloat(data, 'f', 6, 64)
		return sData, nil
	case "float":
		if len(value) == 2 {
			data := float64(binary.BigEndian.Uint16(value))
			sData := strconv.FormatFloat((data * scale), 'f', 6, 64)
			return sData, nil
		} else {
			bits := binary.BigEndian.Uint32(value)
			data := float64(math.Float32frombits(bits)) * scale
			sData := strconv.FormatFloat(data, 'f', 6, 64)
			return sData, nil
		}
	case "boolean":
		return strconv.FormatBool(value[0] == 1), nil
	case "string":
		data := string(value)
		return data, nil

	default:
		return "", errors.New("data type is not support")
	}
}

// Run timer function.
func (td *TwinData) Run() {
	var err error
	data := make([]string, len(td.Name))
	for i := 0; i < len(td.Name); i++ {

		fmt.Printf("\n ---collect [ %s ] data--- \n", td.Name[i])
		//the commandcode include the register address and quatity
		commandcode := [5]byte{}
		switch td.VisitorConfig[i].Register {
		case "CoilRegister": //01
			commandcode[0] = byte(1)
		case "DiscreteInputRegister": //02
			commandcode[0] = byte(2)
		case "HoldingRegister": //03
			commandcode[0] = byte(3)
		case "InputRegister": //04
			commandcode[0] = byte(4)
		default:
			fmt.Println("bad register type")
		}
		commandcode[1] = byte((td.VisitorConfig[i].Offset) >> 8)
		commandcode[2] = byte(td.VisitorConfig[i].Offset)
		commandcode[3] = byte((td.VisitorConfig[i].Limit) >> 8)
		commandcode[4] = byte(td.VisitorConfig[i].Limit)

		results := td.Client.Read(commandcode)
		// transfer data according to the dpl configuration
		sData, err := TransferData(td.VisitorConfig[i].IsRegisterSwap,
			td.VisitorConfig[i].IsSwap, td.Type[i], td.VisitorConfig[i].Scale, results)
		if err != nil {
			klog.Error("Transfer Data failed: ", err)
			return
		}
		data[i] = sData
	}

	// construct payload
	fmt.Printf("\n --publish-- \n")
	var payload []byte
	if strings.Contains(td.Topic, "$hw") {
		if payload, err = common.CreateMessageTwinUpdate(td.Name, td.Type, data); err != nil {
			klog.Error("Create message twin update failed")
			return
		}
	} /*else { //in this project ,the topic about '$ke" is not used
		if payload, err = common.CreateMessageData(td.Name, td.Type, data); err != nil {
			klog.Error("Create message data failed")
			return
		}
	}*/
	if err = globals.MqttClient.Publish(td.Topic, payload); err != nil {
		klog.Errorf("Publish topic %v failed, err: %v", td.Topic, err)
	}
	klog.V(2).Infof("Get the %s value as %s", td.Name, data)
}
