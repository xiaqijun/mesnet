package agent

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Frame format for binary data transfer between agents:
//
//	[4 bytes: total_length (big-endian)]
//	[2 bytes: flags]
//	[16 bytes: target_node_id (hex)]
//	[N bytes: encrypted_payload]

const FrameHeaderSize = 4 + 2 + 16

// EncodeFrame creates a binary frame for sending to a peer.
func EncodeFrame(targetNodeID uint, payload []byte) []byte {
	totalLen := uint32(FrameHeaderSize + len(payload))
	frame := make([]byte, totalLen)

	binary.BigEndian.PutUint32(frame[0:4], totalLen)
	// flags at offset 4-5, reserved for future use
	idStr := fmt.Sprintf("%016d", targetNodeID)
	copy(frame[6:22], idStr)
	copy(frame[22:], payload)

	return frame
}

// DecodeFrame parses a binary frame from a peer.
func DecodeFrame(frame []byte) (uint, uint16, []byte, error) {
	if len(frame) < FrameHeaderSize {
		return 0, 0, nil, io.ErrUnexpectedEOF
	}

	totalLen := binary.BigEndian.Uint32(frame[0:4])
	if uint32(len(frame)) < totalLen {
		return 0, 0, nil, io.ErrUnexpectedEOF
	}

	flags := binary.BigEndian.Uint16(frame[4:6])

	var nodeID uint
	idStr := string(frame[6:22])
	fmt.Sscanf(idStr, "%d", &nodeID)

	payload := make([]byte, int(totalLen)-FrameHeaderSize)
	copy(payload, frame[22:totalLen])

	return nodeID, flags, payload, nil
}
