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
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	"k8s.io/klog/v2"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kubeedge/mappers-go/mappers/common"
	"github.com/kubeedge/mappers-go/mappers/lora/configmap"
	"github.com/kubeedge/mappers-go/mappers/lora/driver"
	"github.com/kubeedge/mappers-go/mappers/lora/globals"
)

var devices map[string]*globals.LoraDev
var models map[string]common.DeviceModel
var protocols map[string]common.Protocol
var wg sync.WaitGroup

// setVisitor check if visitory property is readonly, if not then set it.
func setVisitor(visitorConfig *configmap.LoraVisitorConfig, twin *common.Twin, client *driver.Lora_tcpclient) {
	if twin.PVisitor.PProperty.AccessMode == "ReadOnly" {
		klog.V(1).Info("Visit readonly register: ", visitorConfig.Offset)
		return
	}

	//make the command code include functioncode and register address and data
	commandcode := [5]byte{}
	switch visitorConfig.Register {
	case "HoldingRegister": //06
		commandcode[0] = byte(6)
	default:
		fmt.Println("bad register type")
	}
	commandcode[1] = byte((visitorConfig.Offset) >> 8)
	commandcode[2] = byte(visitorConfig.Offset)

	klog.V(2).Infof("Convert type: %s, value: %s ", twin.PVisitor.PProperty.DataType, twin.Desired.Value)
	value, err := common.Convert(twin.PVisitor.PProperty.DataType, twin.Desired.Value)
	if err != nil {
		klog.Errorf("Convert error: %v", err)
		return
	}
	valueInt, _ := value.(int64)
	commandcode[3] = byte(valueInt >> 8)
	commandcode[4] = byte(valueInt)

	client.Write(commandcode)
}

// getDeviceID extract the device ID from Mqtt topic.
func getDeviceID(topic string) (id string) {
	re := regexp.MustCompile(`hw/events/device/(.+)/twin/update/delta`)
	return re.FindStringSubmatch(topic)[1]
}

// onMessage callback function of Mqtt subscribe message.
func onMessage(client mqtt.Client, message mqtt.Message) {
	klog.V(2).Info("Receive message", message.Topic())
	// Get device ID and get device instance
	id := getDeviceID(message.Topic())
	if id == "" {
		klog.Error("Wrong topic")
		return
	}
	klog.V(2).Info("Device id: ", id)

	var dev *globals.LoraDev
	var ok bool
	if dev, ok = devices[id]; !ok {
		klog.Error("Device not exist")
		return
	}

	// Get twin map key as the propertyName
	var delta common.DeviceTwinDelta
	if err := json.Unmarshal(message.Payload(), &delta); err != nil {
		klog.Errorf("Unmarshal message failed: %v", err)
		return
	}
	for twinName, twinValue := range delta.Delta {
		i := 0
		for i = 0; i < len(dev.Instance.Twins); i++ {
			if twinName == dev.Instance.Twins[i].PropertyName {
				break
			}
		}
		if i == len(dev.Instance.Twins) {
			klog.Error("Twin not found: ", twinName)
			continue
		}
		// Desired value is not changed.
		if dev.Instance.Twins[i].Desired.Value == twinValue {
			continue
		}
		dev.Instance.Twins[i].Desired.Value = twinValue
		var visitorConfig configmap.LoraVisitorConfig
		if err := json.Unmarshal([]byte(dev.Instance.Twins[i].PVisitor.VisitorConfig), &visitorConfig); err != nil {
			klog.Errorf("Unmarshal visitor config failed: %v", err)
			continue
		}
		setVisitor(&visitorConfig, &dev.Instance.Twins[i], dev.TCPClient)
	}
}

// initModbus initialize lora client
func initLora(protocolConfig configmap.LoraProtocolCommonConfig) (client *driver.Lora_tcpclient, err error) {
	fmt.Println("init lora tcp client")
	tcpconfig := driver.Tcpconfig{
		IP:             protocolConfig.IP,
		Port:           protocolConfig.Port,
		ConcentratorID: protocolConfig.ConcentratorID,
		ConverterID:    protocolConfig.ConverterID,
		SlaveID:        protocolConfig.SlaveID,
	}
	client = driver.NewClient(tcpconfig)
	return client, nil
}

// initTwin initialize the timer to get twin value.
func initTwin(dev *globals.LoraDev) {
	//store all the property
	sName := make([]string, len(dev.Instance.Twins))
	sType := make([]string, len(dev.Instance.Twins))
	sVisitorconfig := make([]configmap.LoraVisitorConfig, len(dev.Instance.Twins))

	for i := 0; i < len(dev.Instance.Twins); i++ {
		var visitorConfig configmap.LoraVisitorConfig
		if err := json.Unmarshal([]byte(dev.Instance.Twins[i].PVisitor.VisitorConfig), &visitorConfig); err != nil {
			klog.Errorf("Unmarshal VisitorConfig error: %v", err)
			continue
		}
		setVisitor(&visitorConfig, &dev.Instance.Twins[i], dev.TCPClient)
		sName[i] = dev.Instance.Twins[i].PropertyName
		sType[i] = dev.Instance.Twins[i].Desired.Metadatas.Type
		sVisitorconfig[i] = visitorConfig
		//fmt.Println("180",sVisitorconfig)
	}
	twinData := TwinData{Client: dev.TCPClient,
		Name:          sName,
		Type:          sType,
		VisitorConfig: sVisitorconfig,
		Topic:         fmt.Sprintf(common.TopicTwinUpdate, dev.Instance.ID)}

	//because of we send the twindata which include all property ,so we choose the twin[0]
	collectCycle := time.Duration(dev.Instance.Twins[0].PVisitor.CollectCycle)
	//fmt.Println("name:",dev.Instance.Twins[i].PropertyName,"type:",dev.Instance.Twins[i].Desired.Metadatas.Type)
	// If the collect cycle is not set, set it to 1 second.
	if collectCycle == 0 {
		collectCycle = 1 * time.Second
	}
	timer := common.Timer{Function: twinData.Run, Duration: collectCycle, Times: 0}
	wg.Add(1)
	go func() {
		defer wg.Done()
		timer.Start()
	}()
}

// initSubscribeMqtt subscribe Mqtt topics.
func initSubscribeMqtt(instanceID string) error {
	topic := fmt.Sprintf(common.TopicTwinUpdateDelta, instanceID)
	klog.V(1).Info("Subscribe topic: ", topic)
	return globals.MqttClient.Subscribe(topic, onMessage)
}

// initGetStatus start timer to get device status and send to eventbus.
func initGetStatus(dev *globals.LoraDev) {
	getStatus := GetStatus{Client: dev.TCPClient,
		topic: fmt.Sprintf(common.TopicStateUpdate, dev.Instance.ID)}
	timer := common.Timer{Function: getStatus.Run, Duration: 10 * time.Second, Times: 0}
	wg.Add(1)
	go func() {
		defer wg.Done()
		timer.Start()
	}()
}

// start start the device.
func start(dev *globals.LoraDev) {
	fmt.Println("load protocolconfig")
	var protocolCommConfig configmap.LoraProtocolCommonConfig
	if err := json.Unmarshal([]byte(dev.Instance.PProtocol.ProtocolCommonConfig), &protocolCommConfig); err != nil {
		klog.Errorf("Unmarshal ProtocolCommonConfig error: %v", err)
		return
	}

	client, err := initLora(protocolCommConfig)
	fmt.Println("lora yes")
	if err != nil {
		klog.Errorf("Init error: %v", err)
		return
	}
	dev.TCPClient = client

	initTwin(dev)
	initGetStatus(dev)

	if err := initSubscribeMqtt(dev.Instance.ID); err != nil {
		klog.Errorf("Init subscribe mqtt error: %v", err)
		return
	}

}

// DevInit initialize the device datas.
func DevInit(configmapPath string) error {
	devices = make(map[string]*globals.LoraDev)
	models = make(map[string]common.DeviceModel)
	protocols = make(map[string]common.Protocol)
	return configmap.Parse(configmapPath, devices, models, protocols)
}

// DevStart start all devices.
func DevStart() {
	for id, dev := range devices {
		klog.V(4).Info("Dev: ", id, dev)
		start(dev)
	}
	wg.Wait()
}
