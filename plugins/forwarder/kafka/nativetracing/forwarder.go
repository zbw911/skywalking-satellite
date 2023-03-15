// Licensed to Apache Software Foundation (ASF) under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Apache Software Foundation (ASF) licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package nativetracing

import (
	"fmt"
	"reflect"

	"github.com/Shopify/sarama"
	"google.golang.org/protobuf/proto"

	"github.com/apache/skywalking-satellite/internal/pkg/config"
	"github.com/apache/skywalking-satellite/internal/pkg/log"
	"github.com/apache/skywalking-satellite/internal/satellite/event"

	v3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	v1 "skywalking.apache.org/repo/goapi/satellite/data/v1"
)

const (
	Name     = "native-tracing-kafka-forwarder"
	ShowName = "Native Tracing Kafka Forwarder"
)

type Forwarder struct {
	config.CommonFields
	Topic    string `mapstructure:"topic"` // The forwarder topic.
	producer sarama.SyncProducer
}

func (f *Forwarder) Name() string {
	return Name
}

func (f *Forwarder) ShowName() string {
	return ShowName
}

func (f *Forwarder) Description() string {
	return "This is a synchronization Kafka forwarder with the SkyWalking native log protocol."
}

func (f *Forwarder) DefaultConfig() string {
	return `
# The remote topic. 
topic: "skywalking-segments"
`
}

func (f *Forwarder) Prepare(connection interface{}) error {
	client, ok := connection.(sarama.Client)
	if !ok {
		return fmt.Errorf("the %s only accepts a grpc client, but received a %s",
			f.Name(), reflect.TypeOf(connection).String())
	}
	producer, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		return err
	}
	f.producer = producer
	return nil
}

func (f *Forwarder) Forward(batch event.BatchEvents) error {
	var message []*sarama.ProducerMessage
	for _, e := range batch {
		switch data := e.GetData().(type) {
		case *v1.SniffData_Segment:
			segmentObject := &v3.SegmentObject{}
			err := proto.Unmarshal(data.Segment, segmentObject)
			if err != nil {
				return err
			}

			message = append(message, &sarama.ProducerMessage{
				Topic: f.Topic,
				Key:   sarama.StringEncoder(segmentObject.GetTraceSegmentId()),
				Value: sarama.ByteEncoder(data.Segment),
			})
		case *v1.SniffData_SpanAttachedEvent:
			// SniffData_SpanAttachedEvent is from ebpf agent, skywalking-rover project.
			// You could find it here,
			// https://github.com/apache/skywalking-data-collect-protocol/blob/0da9c8b3e111fb51c9f8854cae16d4519462ecfe
			// /language-agent/Tracing.proto#L244
			// ref: https://github.com/apache/skywalking-satellite/pull/128#discussion_r1136909393
			log.Logger.WithField("pipe", f.PipeName).Warnf("native-tracing-kafka-forwarder " +
				"does not support messages of type SpanAttachedEvent and has discarded them." +
				" Please choose native-tracing-grpc-forwarder as a replacement.")
		default:
			continue
		}
	}
	return f.producer.SendMessages(message)
}

func (f *Forwarder) ForwardType() v1.SniffType {
	return v1.SniffType_TracingType
}

func (f *Forwarder) SyncForward(_ *v1.SniffData) (*v1.SniffData, error) {
	return nil, fmt.Errorf("unsupport sync forward")
}

func (f *Forwarder) SupportedSyncInvoke() bool {
	return false
}
