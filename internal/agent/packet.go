package agent

import (
	"encoding/binary"
	"errors"
	"io"
)

// Frame protocol for data-plane communication between agents.
//
// Wire format (22-byte header + variable payload):
//
//	[0:4]   magic    uint32  "MESH" = 0x4D45354E (big-endian)
//	[4:6]   version  uint16  0x0001
//	[6:8]   flags    uint16  see Flag* constants
//	[8:12]  seq      uint32  message sequence number (monotonic)
//	[12:20] node_id  uint64  target node ID (big-endian)
//	[20:22] pkt_len  uint16  encrypted payload length (0–65535)
//	[22:]            []byte  encrypted payload (ChaCha20-Poly1305 ciphertext + 16-byte tag)

const (
	FrameMagic       = 0x4D45354E // "MESH" in big-endian ASCII
	FrameVersion     = 0x0001
	FrameHeaderSize  = 22
	MaxPayloadSize   = 65535
	MaxFrameSize     = FrameHeaderSize + MaxPayloadSize
	DefaultMTU       = 1500
)

// Flags for the frame header.
const (
	FlagProbe         uint16 = 0x0001
	FlagProbeResponse uint16 = 0x0002
	FlagHandshake     uint16 = 0x0004
	FlagData          uint16 = 0x0008
)

var (
	ErrShortFrame   = errors.New("frame too short")
	ErrBadMagic     = errors.New("bad frame magic")
	ErrBadVersion   = errors.New("unsupported frame version")
	ErrPayloadTooBig = errors.New("payload exceeds maximum size")
)

// FrameHeader represents the parsed header of a data-plane frame.
type FrameHeader struct {
	Magic      uint32
	Version    uint16
	Flags      uint16
	Seq        uint32
	NodeID     uint64
	PayloadLen uint16
}

// EncodeFrame encodes a data-plane frame into a byte slice.
// Uses binary encoding (no fmt) for performance.
func EncodeFrame(flags uint16, seq uint32, nodeID uint64, payload []byte) []byte {
	if len(payload) > MaxPayloadSize {
		payload = payload[:MaxPayloadSize]
	}
	totalLen := FrameHeaderSize + len(payload)
	frame := make([]byte, totalLen)

	binary.BigEndian.PutUint32(frame[0:4], FrameMagic)
	binary.BigEndian.PutUint16(frame[4:6], FrameVersion)
	binary.BigEndian.PutUint16(frame[6:8], flags)
	binary.BigEndian.PutUint32(frame[8:12], seq)
	binary.BigEndian.PutUint64(frame[12:20], nodeID)
	binary.BigEndian.PutUint16(frame[20:22], uint16(len(payload)))
	copy(frame[22:], payload)

	return frame
}

// DecodeFrameHeader parses and validates the header of a data-plane frame.
// Returns the header and the payload slice (reference into frame, no copy).
func DecodeFrameHeader(frame []byte) (FrameHeader, []byte, error) {
	if len(frame) < FrameHeaderSize {
		return FrameHeader{}, nil, ErrShortFrame
	}

	magic := binary.BigEndian.Uint32(frame[0:4])
	if magic != FrameMagic {
		return FrameHeader{}, nil, ErrBadMagic
	}

	ver := binary.BigEndian.Uint16(frame[4:6])
	if ver != FrameVersion {
		return FrameHeader{}, nil, ErrBadVersion
	}

	hdr := FrameHeader{
		Magic:      magic,
		Version:    ver,
		Flags:      binary.BigEndian.Uint16(frame[6:8]),
		Seq:        binary.BigEndian.Uint32(frame[8:12]),
		NodeID:     binary.BigEndian.Uint64(frame[12:20]),
		PayloadLen: binary.BigEndian.Uint16(frame[20:22]),
	}

	end := FrameHeaderSize + int(hdr.PayloadLen)
	if end > len(frame) {
		return FrameHeader{}, nil, io.ErrUnexpectedEOF
	}

	return hdr, frame[FrameHeaderSize:end], nil
}

// WriteFrame encodes a frame and writes it directly to the writer.
// More efficient than EncodeFrame for streaming use.
func WriteFrame(w io.Writer, flags uint16, seq uint32, nodeID uint64, payload []byte) error {
	if len(payload) > MaxPayloadSize {
		return ErrPayloadTooBig
	}

	var buf [FrameHeaderSize]byte
	binary.BigEndian.PutUint32(buf[0:4], FrameMagic)
	binary.BigEndian.PutUint16(buf[4:6], FrameVersion)
	binary.BigEndian.PutUint16(buf[6:8], flags)
	binary.BigEndian.PutUint32(buf[8:12], seq)
	binary.BigEndian.PutUint64(buf[12:20], nodeID)
	binary.BigEndian.PutUint16(buf[20:22], uint16(len(payload)))

	if _, err := w.Write(buf[:]); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return nil
}
