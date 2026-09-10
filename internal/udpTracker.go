package internal

import (
	"encoding/binary"
	"errors"
)

const (
	udpTrackerProtocolConstant uint64 = 0x41727101980
)

const (
	actionConnect uint32 = iota
	actionAnnounce
	actionScrape
	actionError
)

type connectionRequest struct {
	// Randomly selected transaction ID
	transactionID uint32
}

type connectionResponse struct {
	action        uint32
	transactionID uint32
	connectionID  uint64
}

func (req *connectionRequest) marshalRequest() []byte {
	/*
		Offset  Size            Name            Value
		0       64-bit integer  protocol_id     0x41727101980 // magic constant
		8       32-bit integer  action          0 // connect
		12      32-bit integer  transaction_id
		16
	*/
	data := make([]byte, 16)
	binary.BigEndian.PutUint64(data[0:], udpTrackerProtocolConstant)
	binary.BigEndian.PutUint32(data[8:], actionConnect)
	binary.BigEndian.PutUint32(data[12:], req.transactionID)
	return data
}

func (res *connectionResponse) unmarshalResponse(response []byte) error {
	/*
		Offset  Size            Name            Value
		0       32-bit integer  action          0 // connect
		4       32-bit integer  transaction_id
		8       64-bit integer  connection_id
		16
	*/
	if len(response) < 16 {
		return udpTrackerConnectResponseTooShort
	}
	res.action = binary.BigEndian.Uint32(response[0:4])
	res.transactionID = binary.BigEndian.Uint32(response[4:8])
	res.connectionID = binary.BigEndian.Uint64(response[8:16])
	return nil
}

func (res *connectionResponse) validateAgainstRequest(request *connectionRequest) error {
	if res.transactionID != request.transactionID {
		return udpTrackerMismatchedTransactionID
	}
	if res.action != actionConnect {
		return udpTrackerInvalidResponseAction
	}
	return nil
}

type announceReq struct {
	connectionID  uint64
	action        uint32
	transactionID uint32
	infoHash      hashBytes
	peerID        peerID
	downloaded    uint64
	left          uint64
	uploaded      uint64
	event         uint32 // 0: none; 1: completed; 2: started; 3: stopped
	key           uint32 // unique identified for if client's IP address changes
	numWant       uint32 // number of peers wanted. -1  default
	port          uint16 // port client is listening on for connections
}

type announceResponse struct {
	action             uint32
	transactionID      uint32
	interval           uint32
	leechers           uint32
	seeders            uint32
	peerAddressesBytes []byte
}

func (req *announceReq) marshalRequest() []byte {
	/*
		0       64-bit integer  connection_id
		8       32-bit integer  action          1 // announce
		12      32-bit integer  transaction_id
		16      20-byte string  info_hash
		36      20-byte string  peer_id
		56      64-bit integer  downloaded
		64      64-bit integer  left
		72      64-bit integer  uploaded
		80      32-bit integer  event           0 // 0: none; 1: completed; 2: started; 3: stopped
		84      32-bit integer  IP address      0 // default
		88      32-bit integer  key
		92      32-bit integer  num_want        -1 // default
		96      16-bit integer  port
		98
	*/
	reqBytes := make([]byte, 98)
	binary.BigEndian.PutUint64(reqBytes[0:], req.connectionID)
	binary.BigEndian.PutUint32(reqBytes[8:], req.action)
	binary.BigEndian.PutUint32(reqBytes[12:], req.transactionID)
	copy(reqBytes[16:], req.infoHash[:])
	copy(reqBytes[36:], req.peerID[:])
	binary.BigEndian.PutUint64(reqBytes[56:], req.downloaded)
	binary.BigEndian.PutUint64(reqBytes[64:], req.left)
	binary.BigEndian.PutUint64(reqBytes[72:], req.uploaded)
	binary.BigEndian.PutUint32(reqBytes[80:], req.event)
	binary.BigEndian.PutUint32(reqBytes[84:], 0)
	binary.BigEndian.PutUint32(reqBytes[88:], req.key)
	binary.BigEndian.PutUint32(reqBytes[92:], req.numWant)
	binary.BigEndian.PutUint16(reqBytes[96:], req.port)
	return reqBytes
}

func (res *announceResponse) unmarshalResponse(response []byte) error {
	/*
		Offset      Size            Name            Value
		0           32-bit integer  action          1 // announce
		4           32-bit integer  transaction_id
		8           32-bit integer  interval
		12          32-bit integer  leechers
		16          32-bit integer  seeders
		20 + 6 * n  32-bit integer  IP address
		24 + 6 * n  16-bit integer  TCP port
		20 + 6 * N
	*/
	if len(response) < 16 {
		return udpTrackerAnnounceResponseTooShort
	}
	res.action = binary.BigEndian.Uint32(response[0:4])
	res.transactionID = binary.BigEndian.Uint32(response[4:8])
	res.interval = binary.BigEndian.Uint32(response[8:12])
	res.leechers = binary.BigEndian.Uint32(response[12:16])
	res.seeders = binary.BigEndian.Uint32(response[16:20])
	res.peerAddressesBytes = response[20:]
	return nil
}

func (res *announceResponse) validateAgainstRequest(request *announceReq) error {
	if res.transactionID != request.transactionID {
		return udpTrackerMismatchedTransactionID
	}
	if res.action != actionAnnounce {
		return udpTrackerInvalidResponseAction
	}
	return nil
}

// TODO: Better errors - by wrapping perhaps.
var (
	udpTrackerConnectResponseTooShort  = errors.New("UDP tracker connect response too short")
	udpTrackerMismatchedTransactionID  = errors.New("UDP tracker mismatched transaction ID")
	udpTrackerInvalidResponseAction    = errors.New("UDP tracker invalid response action")
	udpTrackerAnnounceResponseTooShort = errors.New("UDP tracker announce response too short")
)
