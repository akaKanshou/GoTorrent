package internal

const (
	tpEventNewPeer = iota
	tpEventDestroyPeer
	tpEventIncomingMessage
	tpPieceResponsibilityReqEvent
)

type event interface {
	eventType() int
}

type newPeerEvent struct {
	newPeer *peer
}

func (e *newPeerEvent) eventType() int { return tpEventNewPeer }

type destroyPeerEvent struct {
	peer *peer
}

func (e *destroyPeerEvent) eventType() int { return tpEventDestroyPeer }

type incomingMessageEvent struct {
	peer    *peer
	message peerwireMessage
}

func (e *incomingMessageEvent) eventType() int { return tpEventIncomingMessage }

type pieceResponsibilityReqEvent struct {
	peer *peer
}

func (e *pieceResponsibilityReqEvent) eventType() int { return tpPieceResponsibilityReqEvent }
