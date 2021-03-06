// Copyright 2018 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nftables

import (
	"encoding/binary"
	"fmt"

	"github.com/SewanDevs/netlink"
	"golang.org/x/sys/unix"
)

var objHeaderType = netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_NEWOBJ)

// Obj represents a netfilter stateful object. See also
// https://wiki.nftables.org/wiki-nftables/index.php/Stateful_objects
type Obj interface {
	family() TableFamily
	unmarshal(*netlink.AttributeDecoder) error
	marshal(data bool) ([]byte, error)
}

// AddObj adds the specified Obj. See also
// https://wiki.nftables.org/wiki-nftables/index.php/Stateful_objects
func (cc *Conn) AddObj(o Obj) Obj {
	data, err := o.marshal(true)
	if err != nil {
		cc.setErr(err)
		return nil
	}

	cc.messages = append(cc.messages, netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_NEWOBJ),
			Flags: netlink.Request | netlink.Acknowledge | netlink.Create,
		},
		Data: append(extraHeader(uint8(o.family()), 0), data...),
	})
	return o
}

// GetObj gets the specified Obj without resetting it.
func (cc *Conn) GetObj(o Obj) ([]Obj, error) {
	return cc.getObj(o, unix.NFT_MSG_GETOBJ)
}

// GetObjReset gets the specified Obj and resets it.
func (cc *Conn) GetObjReset(o Obj) ([]Obj, error) {
	return cc.getObj(o, unix.NFT_MSG_GETOBJ_RESET)
}

func objFromMsg(msg netlink.Message) (Obj, error) {
	if got, want := msg.Header.Type, objHeaderType; got != want {
		return nil, fmt.Errorf("unexpected header type: got %v, want %v", got, want)
	}
	ad, err := netlink.NewAttributeDecoder(msg.Data[4:])
	if err != nil {
		return nil, err
	}
	ad.ByteOrder = binary.BigEndian
	var (
		table      *Table
		name       string
		objectType uint32
	)
	const NFT_OBJECT_COUNTER = 1 // TODO: get into x/sys/unix
	for ad.Next() {
		switch ad.Type() {
		case unix.NFTA_OBJ_TABLE:
			table = &Table{Name: ad.String(), Family: TableFamily(msg.Data[0])}
		case unix.NFTA_OBJ_NAME:
			name = ad.String()
		case unix.NFTA_OBJ_TYPE:
			objectType = ad.Uint32()
		case unix.NFTA_OBJ_DATA:
			switch objectType {
			case NFT_OBJECT_COUNTER:
				o := CounterObj{
					Table: table,
					Name:  name,
				}

				ad.Do(func(b []byte) error {
					ad, err := netlink.NewAttributeDecoder(b)
					if err != nil {
						return err
					}
					ad.ByteOrder = binary.BigEndian
					return o.unmarshal(ad)
				})
				return &o, ad.Err()
			}
		}
	}
	if err := ad.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("malformed stateful object")
}

func (cc *Conn) getObj(o Obj, msgType uint16) ([]Obj, error) {
	conn, err := cc.dialNetlink()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	data, err := o.marshal(false)
	if err != nil {
		return nil, err
	}

	message := netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | msgType),
			Flags: netlink.Request | netlink.Acknowledge | netlink.Dump,
		},
		Data: append(extraHeader(uint8(o.family()), 0), data...),
	}

	if _, err := conn.SendMessages([]netlink.Message{message}); err != nil {
		return nil, fmt.Errorf("SendMessages: %v", err)
	}

	reply, err := conn.Receive()
	if err != nil {
		return nil, fmt.Errorf("Receive: %v", err)
	}
	var objs []Obj
	for _, msg := range reply {
		o, err := objFromMsg(msg)
		if err != nil {
			return nil, err
		}
		objs = append(objs, o)
	}

	return objs, nil
}
