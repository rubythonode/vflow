// Package hammer generates ipfix packets
//: ----------------------------------------------------------------------------
//: Copyright (C) 2017 Verizon.  All Rights Reserved.
//: All Rights Reserved
//:
//: file:    hammer.go
//: details: TODO
//: author:  Mehrdad Arshad Rad
//: date:    02/01/2017
//:
//: Licensed under the Apache License, Version 2.0 (the "License");
//: you may not use this file except in compliance with the License.
//: You may obtain a copy of the License at
//:
//:     http://www.apache.org/licenses/LICENSE-2.0
//:
//: Unless required by applicable law or agreed to in writing, software
//: distributed under the License is distributed on an "AS IS" BASIS,
//: WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//: See the License for the specific language governing permissions and
//: limitations under the License.
//: ----------------------------------------------------------------------------
package hammer

import (
	"fmt"
	"net"
	"sync"
	"time"

	"git.edgecastcdn.net/vflow/mirror"
)

const (
	dataType = iota
	templateType
	templateOptType
)

type Packet struct {
	payload []byte
	length  int
}

type IPFIX struct {
	conn mirror.Conn
	ch   chan Packet
	srcs []net.IP

	MaxRouter   int
	TplInterval time.Duration
	IPFIXAddr   net.IP
	IPFIXPort   int
	RateLimit   int
}

func NewIPFIX() (*IPFIX, error) {

	raddr := net.ParseIP("127.0.0.1")
	conn, err := mirror.NewRawConn(raddr)
	if err != nil {
		return nil, err
	}

	return &IPFIX{
		conn:        conn,
		ch:          make(chan Packet, 10000),
		MaxRouter:   10,
		TplInterval: 10 * time.Second,
	}, nil
}

func (i *IPFIX) Run() {
	var wg sync.WaitGroup

	for j := 1; j < i.MaxRouter; j++ {
		i.srcs = append(i.srcs, net.ParseIP(fmt.Sprintf("192.168.1.%d", j)))
	}

	wg.Add(1)
	go func() {
		var p Packet
		defer wg.Done()
		for {
			p = <-i.ch
			i.conn.Send(p.payload[:p.length])
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		i.sendTemplate()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		i.sendTemplateOpt()
	}()

	time.Sleep(1 * time.Second)

	wg.Add(1)
	go func() {
		defer wg.Done()
		i.sendData()
	}()

	wg.Wait()
}

func (i *IPFIX) sendData() {
	packets := i.genPackets(dataType)

	for {
		for j, _ := range packets {
			i.ch <- packets[j]
		}
	}
}

func (i *IPFIX) sendTemplate() {
	c := time.Tick(i.TplInterval)
	packets := i.genPackets(templateType)

	for j, _ := range packets {
		i.ch <- packets[j]
	}

	for range c {
		for j, _ := range packets {
			i.ch <- packets[j]
		}
	}
}

func (i *IPFIX) sendTemplateOpt() {
	c := time.Tick(i.TplInterval)
	packets := i.genPackets(templateOptType)

	for j, _ := range packets {
		i.ch <- packets[j]
	}

	for range c {
		for j, _ := range packets {
			i.ch <- packets[j]
		}
	}
}

func (i *IPFIX) genPackets(typ int) []Packet {
	var (
		packets []Packet
		samples [][]byte
	)

	ipHLen := mirror.IPv4HLen
	udp := mirror.UDP{55117, 4739, 0, 0}
	udpHdr := udp.Marshal()

	ip := mirror.NewIPv4HeaderTpl(mirror.UDPProto)
	ipHdr := ip.Marshal()

	switch typ {
	case dataType:
		samples = ipfixDataSamples
	case templateType:
		samples = ipfixTemplates
	case templateOptType:
		samples = ipfixTemplatesOpt
	}

	for j := 0; j < len(samples); j++ {
		for _, src := range i.srcs {
			data := samples[j]
			payload := make([]byte, 1500)

			udp.SetLen(udpHdr, len(data))

			ip.SetAddrs(ipHdr, src, net.ParseIP("127.0.0.1"))

			copy(payload[0:ipHLen], ipHdr)
			copy(payload[ipHLen:ipHLen+8], udpHdr)
			copy(payload[ipHLen+8:], data)

			packets = append(packets, Packet{
				payload: payload,
				length:  ipHLen + 8 + len(data),
			})

		}
	}

	return packets
}