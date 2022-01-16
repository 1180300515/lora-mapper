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

package configmap

// LoraVisitorConfig is the modbus register configuration.
type LoraVisitorConfig struct {
	Register       string  `json:"register"`
	Offset         uint16  `json:"offset"`
	Limit          int     `json:"limit"`
	Scale          float64 `json:"scale,omitempty"`
	IsSwap         bool    `json:"isSwap,omitempty"`
	IsRegisterSwap bool    `json:"isRegisterSwap,omitempty"`
}

// LoraProtocolCommonConfig is the lora-tcp configuration.
type LoraProtocolCommonConfig struct {
	IP             string `json:"ip"`
	Port           string `json:"port"`
	ConcentratorID uint32 `json:"concentratorID,omitempty"`
	ConverterID    uint32 `json:"converterID,omitempty"`
	SlaveID        int16  `json:"slaveID,omitempty"`
}
