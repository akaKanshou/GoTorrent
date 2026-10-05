package internal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand"
	"time"

	"gitlab.com/akaKanshou/gotorrent/logger"
)

const (
	pStr    = "BitTorrent protocol"
	lenPStr = len(pStr)

	infoHashOffset  = lenPStr + 9
	peerIDOffset    = infoHashOffset + 20
	handshakeLength = 49 + lenPStr
)

const (
	maxRequestLength = 1 << 14
)

const (
	peerwireChokeID byte = iota
	peerwireUnchokeID
	peerwireInterestedID
	peerwireNotInterestedID
	peerwireHaveID
	peerwireBitfieldID
	peerwireRequestID
	peerwirePieceID
	peerwireCancelID
	peerwirePortID
	peerwireKeepAliveID  // Not defined by spec - only for the application's use
	peerwireMultiMessage // Not defined by spec - only for the application's use
)

// <pstrlen><pstr><reserved><info_hash><peer_id>
type handshakeMessage struct {
	// The peerID of the client/peer
	peerID peerID

	// The infoHash of the torrent
	infoHash hashBytes

	// The extensions supported by the client/peer
	options uint64
}

func (h *handshakeMessage) marshalMessage() []byte {
	message := make([]byte, 49+lenPStr)

	message[0] = byte(lenPStr)
	copy(message[1:], pStr)

	binary.BigEndian.PutUint64(message[1+lenPStr:], h.options)

	copy(message[infoHashOffset:], h.infoHash[:])
	copy(message[peerIDOffset:], h.peerID[:])
	return message
}

func (h *handshakeMessage) unmarshalMessage(handshake []byte) error {
	if len(handshake) != handshakeLength {
		return handshakeInvalidLengthError
	}
	pLenPStr := handshake[0]
	if int(pLenPStr) != lenPStr { // protocol constant
		return handshakeInvalidLengthError
	}
	if string(handshake[1:1+lenPStr]) != pStr {
		return handshakeInvalidPStrError
	}
	h.options = binary.BigEndian.Uint64(handshake[1+lenPStr:])
	copy(h.infoHash[:], handshake[infoHashOffset:infoHashOffset+20])
	copy(h.peerID[:], handshake[peerIDOffset:peerIDOffset+20])
	return nil
}

func (h *handshakeMessage) validateAgainstHandshake(init *handshakeMessage) error {
	if bytes.Compare(init.infoHash[:], h.infoHash[:]) != 0 {
		return handshakeMismatchInfoHashError
	}
	return nil
}

// A peerwire message
type peerwireMessage interface {
	// Returns the base type of peerwireMessage
	// It should be safe to cast directly to the underlying type if the message type is known.
	messageType() byte

	// Makes the corresponding message byte array.
	marshalMessage() []byte

	// Unmarshals payload data into the corresponding peerwire message.
	// Populates the struct's data members
	unmarshalMessage(payload []byte) error
}

func makeMessage(id byte, data []byte) []byte {
	// <len=XXXX><id=X><payload>
	message := make([]byte, 5+len(data))

	binary.BigEndian.PutUint32(message[0:], uint32(len(data)+1)) // +1 for id
	message[4] = id
	copy(message[5:], data)

	return message
}

// keep-alive: <len=0000>
type keepAliveMessage struct {
	message []byte
}

func (m *keepAliveMessage) messageType() byte {
	return peerwireKeepAliveID
}

func (m *keepAliveMessage) marshalMessage() []byte {
	return make([]byte, 4)
}

func (m *keepAliveMessage) unmarshalMessage(payload []byte) error {
	if len(payload) > 0 {
		return invalidMessageLengthError
	}
	return nil
}

// choke: <len=0001><id=0>
type chokeMessage struct {
}

func (c *chokeMessage) messageType() byte {
	return peerwireChokeID
}

func (c *chokeMessage) marshalMessage() []byte {
	return makeMessage(peerwireChokeID, make([]byte, 0))
}

func (c *chokeMessage) unmarshalMessage(payload []byte) error {
	if len(payload) > 0 {
		return invalidMessageLengthError
	}
	return nil
}

// unchoke: <len=0001><id=1>
type unchokeMessage struct {
}

func (u *unchokeMessage) messageType() byte {
	return peerwireUnchokeID
}

func (u *unchokeMessage) marshalMessage() []byte {
	return makeMessage(peerwireUnchokeID, make([]byte, 0))
}

func (u *unchokeMessage) unmarshalMessage(payload []byte) error {
	if len(payload) > 0 {
		return invalidMessageLengthError
	}
	return nil
}

// interested: <len=0001><id=2>
type interestedMessage struct {
}

func (i *interestedMessage) messageType() byte {
	return peerwireInterestedID
}

func (i *interestedMessage) marshalMessage() []byte {
	return makeMessage(peerwireInterestedID, make([]byte, 0))
}

func (i *interestedMessage) unmarshalMessage(payload []byte) error {
	if len(payload) > 0 {
		return invalidMessageLengthError
	}
	return nil
}

// not interested: <len=0001><id=3>
type notInterestedMessage struct {
}

func (n *notInterestedMessage) messageType() byte {
	return peerwireNotInterestedID
}

func (n *notInterestedMessage) marshalMessage() []byte {
	return makeMessage(peerwireNotInterestedID, make([]byte, 0))
}

func (n *notInterestedMessage) unmarshalMessage(payload []byte) error {
	if len(payload) > 0 {
		return invalidMessageLengthError
	}
	return nil
}

// have: <len=0005><id=4><piece index=XXXX>
type haveMessage struct {
	pieceIndex int
}

func (h *haveMessage) messageType() byte {
	return peerwireHaveID
}

func (h *haveMessage) marshalMessage() []byte {
	message := make([]byte, 4)
	binary.BigEndian.PutUint32(message[0:], uint32(h.pieceIndex))

	return makeMessage(peerwireHaveID, message)
}

func (h *haveMessage) unmarshalMessage(payload []byte) error {
	if len(payload) != 4 {
		return invalidMessageLengthError
	}
	h.pieceIndex = int(binary.BigEndian.Uint32(payload[:]))
	return nil
}

// bitfield: <len=0001+Y><id=5><bitfield of len Y>
type bitfieldMessage struct {
	bitfieldData []byte
}

func (b *bitfieldMessage) messageType() byte {
	return peerwireBitfieldID
}

func (b *bitfieldMessage) marshalMessage() []byte {
	return makeMessage(peerwireBitfieldID, b.bitfieldData)
}

func (b *bitfieldMessage) unmarshalMessage(payload []byte) error {
	b.bitfieldData = payload
	return nil
}

func (b *bitfieldMessage) isValid(numPieces int) error {
	if len(b.bitfieldData)*8 < numPieces {
		return bitfieldLengthTooShortError
	}
	for i := numPieces; i < len(b.bitfieldData)*8; i++ {
		// i-th bit -> at floor(i/8)-th byte
		// MSB is filled first, so mask 0x80>>(i%8)
		if b.bitfieldData[i/8]&(0x80>>(i%8)) != 0 {
			return bitfieldSpareBitSetError
		}
	}
	return nil
}

// request: <len=0013><id=6><index=XXXX><begin=XXXX><length=XXXX>
type requestMessage struct {
	// Index of requested piece
	index int

	// The offset within the requested piece
	begin int

	// The size of the requested block.
	// This must be <= 2^14
	length int
}

func (r *requestMessage) messageType() byte {
	return peerwireRequestID
}

func (r *requestMessage) marshalMessage() []byte {
	message := make([]byte, 12)
	binary.BigEndian.PutUint32(message[0:], uint32(r.index))
	binary.BigEndian.PutUint32(message[4:], uint32(r.begin))
	binary.BigEndian.PutUint32(message[8:], uint32(r.length))
	return makeMessage(peerwireRequestID, message)
}

func (r *requestMessage) unmarshalMessage(payload []byte) error {
	if len(payload) != 12 {
		return invalidMessageLengthError
	}
	r.index = int(binary.BigEndian.Uint32(payload))
	r.begin = int(binary.BigEndian.Uint32(payload[4:]))
	r.length = int(binary.BigEndian.Uint32(payload[8:]))
	return nil
}

// piece: <len=0009+Y><id=7><index=XXXX><begin=XXXX><block of len Y>
type pieceMessage struct {
	// Index of the piece
	index int

	// The offset within the piece
	begin int

	// Byte data of the specified block
	block []byte

	// Time it took for the message to be transmitted
	transmissionTime time.Duration
}

func (p *pieceMessage) messageType() byte {
	return peerwirePieceID
}

func (p *pieceMessage) marshalMessage() []byte {
	message := make([]byte, 8+len(p.block))
	binary.BigEndian.PutUint32(message[0:], uint32(p.index))
	binary.BigEndian.PutUint32(message[4:], uint32(p.begin))
	copy(message[8:], p.block)
	return makeMessage(peerwirePieceID, message)
}

func (p *pieceMessage) unmarshalMessage(payload []byte) error {
	p.index = int(binary.BigEndian.Uint32(payload))
	p.begin = int(binary.BigEndian.Uint32(payload[4:]))
	p.block = make([]byte, len(payload[8:]))
	copy(p.block, payload[8:])
	return nil
}

// cancel: <len=0013><id=8><index=XXXX><begin=XXXX><length=XXXX>
type cancelMessage struct {
	// Index of requested piece
	index int

	// The offset within the requested piece
	begin int

	// The size of the requested block.
	length int
}

func (c *cancelMessage) messageType() byte {
	return peerwireCancelID
}

func (c *cancelMessage) marshalMessage() []byte {
	message := make([]byte, 12)
	binary.BigEndian.PutUint32(message[0:], uint32(c.index))
	binary.BigEndian.PutUint32(message[4:], uint32(c.begin))
	binary.BigEndian.PutUint32(message[8:], uint32(c.length))
	return makeMessage(peerwireCancelID, message)
}

func (c *cancelMessage) unmarshalMessage(payload []byte) error {
	if len(payload) != 12 {
		return invalidMessageLengthError
	}
	c.index = int(binary.BigEndian.Uint32(payload))
	c.begin = int(binary.BigEndian.Uint32(payload[4:]))
	c.length = int(binary.BigEndian.Uint32(payload[8:]))
	return nil
}

// The port message is sent by newer versions of the Mainline that implements a DHT tracker.
// The listen port is the port this peer's DHT node is listening on.
// This peer should be inserted in the local routing table (if DHT tracker is supported).
// port: <len=0003><id=9><listen-port=XX>
type portMessage struct {
	port uint16
}

func (p *portMessage) messageType() byte {
	return peerwirePortID
}

func (p *portMessage) marshalMessage() []byte {
	message := make([]byte, 2)
	binary.BigEndian.PutUint16(message, p.port)
	return makeMessage(peerwirePortID, message)
}

func (p *portMessage) unmarshalMessage(payload []byte) error {
	if len(payload) != 2 {
		return invalidMessageLengthError
	}
	p.port = binary.BigEndian.Uint16(payload)
	return nil
}

var (
	invalidMessageLengthError = errors.New("invalid message length")
	invalidMessageIDError     = errors.New("invalid message ID")

	handshakeInvalidLengthError    = errors.New("invalid handshake length")
	handshakeInvalidPStrError      = errors.New("invalid handshake pStr")
	handshakeMismatchInfoHashError = errors.New("handshake mismatch hash")

	bitfieldSpareBitSetError    = errors.New("bitfield spare bit set")
	bitfieldLengthTooShortError = errors.New("bitfield length too short, does not cover all pieces")
)

const (
	maximumRequestLength = 1 << 14
)

// parseMessage takes a message as an input and returns a peerwireMessage.
// Returns an error if the message structure is invalid. Does NOT check the validity of the payload.
// Checks like spare bits of bitfields being OFF are not performed here.
func parseMessage(message []byte, downloadTime time.Duration) (peerwireMessage, error) {
	if len(message) < 4 {
		// Can't have length prefix
		return nil, invalidMessageLengthError
	}

	var messageLength = int(binary.BigEndian.Uint32(message[0:]))
	// <4-byte length prefix><1-byte message ID><X-byte payload>
	if messageLength+4 != len(message) {
		// Prefix length does not match actual length
		return nil, invalidMessageLengthError
	}

	if messageLength == 0 {
		// keep-alive: <len=0000>
		return &keepAliveMessage{}, nil
	}

	messageID := message[4]

	var pwMessage peerwireMessage
	switch messageID {
	case peerwireChokeID:
		pwMessage = &chokeMessage{}
		break
	case peerwireUnchokeID:
		pwMessage = &unchokeMessage{}
		break
	case peerwireInterestedID:
		pwMessage = &interestedMessage{}
		break
	case peerwireNotInterestedID:
		pwMessage = &notInterestedMessage{}
		break
	case peerwireHaveID:
		pwMessage = &haveMessage{}
		break
	case peerwireBitfieldID:
		pwMessage = &bitfieldMessage{}
		break
	case peerwireRequestID:
		pwMessage = &requestMessage{}
		break
	case peerwirePieceID:
		pwMessage = &pieceMessage{
			transmissionTime: downloadTime,
		}
		logger.Logger.Debug("piece received")
		break
	case peerwireCancelID:
		pwMessage = &cancelMessage{}
		break
	case peerwirePortID:
		pwMessage = &portMessage{}
		break
	default:
		return nil, invalidMessageIDError
	}

	payload := message[5:]
	err := pwMessage.unmarshalMessage(payload)
	if err != nil {
		return nil, err
	}
	return pwMessage, nil
}

const (
	clientID            = "GT"
	majorVersionNumber  = "1"
	minorVersionNumber  = "2"
	bugfixVersionNumber = "1" //Hexadecimal

	letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

func randStringBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return b
}

func peerIDGenerate() peerID {
	//Generates peerID following Azereus-Style
	//https://wiki.theory.org/BitTorrentSpecification#peer_id

	var peerID peerID

	prefix := "-" + clientID + majorVersionNumber + minorVersionNumber + bugfixVersionNumber + "0-"
	copy(peerID[:], prefix)
	copy(peerID[len(prefix):], randStringBytes(20-len(prefix)))

	return peerID
}

type multiMessage struct {
	messages []peerwireMessage
}

func (m *multiMessage) messageType() byte {
	return peerwireMultiMessage
}

func (m *multiMessage) marshalMessage() []byte {
	payload := make([]byte, 0)
	for _, message := range m.messages {
		if message != nil {
			payload = append(payload, message.marshalMessage()...)
		}
	}
	return payload
}

func (m *multiMessage) unmarshalMessage(payload []byte) error {
	return undefinedUnmarshalError
}

func newPipedRequestsMessage(index, pieceLength int64) *multiMessage {
	messages := make([]peerwireMessage, 0)
	for offset := 0; offset < int(pieceLength); offset += maxRequestLength {
		messages = append(messages, &requestMessage{
			index:  int(index),
			begin:  offset,
			length: min(maxRequestLength, int(pieceLength)-offset),
		})
	}
	return &multiMessage{messages: messages}
}

var (
	undefinedUnmarshalError = errors.New("undefined unmarshal type")
)
