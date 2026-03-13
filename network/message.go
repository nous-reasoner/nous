package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"nous/crypto"
)

// EncodeMessage serializes a Message to the wire format:
//
//	magic(4) + command(12) + payload_len(4) + checksum(4) + payload
func EncodeMessage(magic uint32, msg Message) ([]byte, error) {
	payload, err := encodePayload(msg)
	if err != nil {
		return nil, fmt.Errorf("network: encode payload: %w", err)
	}
	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf("network: payload too large (%d > %d)", len(payload), MaxPayloadSize)
	}

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, magic)

	// Command: 12 bytes, null-padded.
	var cmd [CommandSize]byte
	copy(cmd[:], msg.Command())
	buf.Write(cmd[:])

	binary.Write(&buf, binary.LittleEndian, uint32(len(payload)))

	// Checksum: first 4 bytes of DoubleSha256(payload).
	checksum := crypto.DoubleSha256(payload)
	buf.Write(checksum[:4])

	buf.Write(payload)
	return buf.Bytes(), nil
}

// DecodeMessage reads a single message from a reader.
func DecodeMessage(r io.Reader, expectedMagic uint32) (Message, error) {
	// Read header.
	var hdr [HeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("network: read header: %w", err)
	}

	magic := binary.LittleEndian.Uint32(hdr[0:4])
	if magic != expectedMagic {
		return nil, fmt.Errorf("network: bad magic %08x (expected %08x)", magic, expectedMagic)
	}

	cmd := string(bytes.TrimRight(hdr[4:16], "\x00"))
	payloadLen := binary.LittleEndian.Uint32(hdr[16:20])
	if payloadLen > MaxPayloadSize {
		return nil, fmt.Errorf("network: payload too large (%d)", payloadLen)
	}
	var checksumWant [4]byte
	copy(checksumWant[:], hdr[20:24])

	// Read payload.
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("network: read payload: %w", err)
		}
	}

	// Verify checksum.
	checksumGot := crypto.DoubleSha256(payload)
	if checksumWant != [4]byte(checksumGot[:4]) {
		return nil, fmt.Errorf("network: checksum mismatch")
	}

	return decodePayload(cmd, payload)
}

// --- payload encoding ---

func encodePayload(msg Message) ([]byte, error) {
	var buf bytes.Buffer

	switch m := msg.(type) {
	case *MsgVersion:
		binary.Write(&buf, binary.LittleEndian, m.Version)
		binary.Write(&buf, binary.LittleEndian, m.BlockHeight)
		binary.Write(&buf, binary.LittleEndian, m.Timestamp)
		binary.Write(&buf, binary.LittleEndian, m.Nonce)
		binary.Write(&buf, binary.LittleEndian, m.ListenPort)
		writeString(&buf, m.UserAgent)

	case *MsgVerAck:
		// no payload

	case *MsgGetBlocks:
		buf.Write(m.StartHash[:])
		buf.Write(m.StopHash[:])

	case *MsgInv:
		binary.Write(&buf, binary.LittleEndian, uint32(len(m.Items)))
		for _, item := range m.Items {
			binary.Write(&buf, binary.LittleEndian, uint32(item.Type))
			buf.Write(item.Hash[:])
		}

	case *MsgGetData:
		binary.Write(&buf, binary.LittleEndian, uint32(len(m.Items)))
		for _, item := range m.Items {
			binary.Write(&buf, binary.LittleEndian, uint32(item.Type))
			buf.Write(item.Hash[:])
		}

	case *MsgBlock:
		buf.Write(m.Payload)

	case *MsgTx:
		buf.Write(m.Payload)

	case *MsgPing:
		binary.Write(&buf, binary.LittleEndian, m.Nonce)

	case *MsgPong:
		binary.Write(&buf, binary.LittleEndian, m.Nonce)

	case *MsgAddr:
		binary.Write(&buf, binary.LittleEndian, uint32(len(m.Addresses)))
		for _, addr := range m.Addresses {
			writeString(&buf, addr.IP)
			binary.Write(&buf, binary.LittleEndian, addr.Port)
		}

	case *MsgGetAddr:
		// empty payload

	case *MsgGetHeaders:
		buf.Write(m.StartHash[:])
		buf.Write(m.StopHash[:])

	case *MsgHeaders:
		buf.Write(m.Headers)

	default:
		return nil, fmt.Errorf("network: unknown message type %T", msg)
	}

	return buf.Bytes(), nil
}

func decodePayload(cmd string, payload []byte) (Message, error) {
	r := bytes.NewReader(payload)

	switch cmd {
	case CmdVersion:
		m := &MsgVersion{}
		binary.Read(r, binary.LittleEndian, &m.Version)
		binary.Read(r, binary.LittleEndian, &m.BlockHeight)
		binary.Read(r, binary.LittleEndian, &m.Timestamp)
		binary.Read(r, binary.LittleEndian, &m.Nonce)
		binary.Read(r, binary.LittleEndian, &m.ListenPort)
		var err error
		m.UserAgent, err = readString(r)
		if err != nil {
			return nil, err
		}
		return m, nil

	case CmdVerAck:
		return &MsgVerAck{}, nil

	case CmdGetBlocks:
		m := &MsgGetBlocks{}
		io.ReadFull(r, m.StartHash[:])
		io.ReadFull(r, m.StopHash[:])
		return m, nil

	case CmdInv:
		return decodeInvList(r, func(items []InvItem) Message { return &MsgInv{Items: items} })

	case CmdGetData:
		return decodeInvList(r, func(items []InvItem) Message { return &MsgGetData{Items: items} })

	case CmdBlock:
		return &MsgBlock{Payload: payload}, nil

	case CmdTx:
		return &MsgTx{Payload: payload}, nil

	case CmdPing:
		m := &MsgPing{}
		binary.Read(r, binary.LittleEndian, &m.Nonce)
		return m, nil

	case CmdPong:
		m := &MsgPong{}
		binary.Read(r, binary.LittleEndian, &m.Nonce)
		return m, nil

	case CmdAddr:
		var count uint32
		binary.Read(r, binary.LittleEndian, &count)
		if count > MaxAddrCount {
			return nil, fmt.Errorf("network: addr count %d exceeds max %d", count, MaxAddrCount)
		}
		addrs := make([]NetAddress, count)
		for i := uint32(0); i < count; i++ {
			ip, err := readString(r)
			if err != nil {
				return nil, err
			}
			addrs[i].IP = ip
			binary.Read(r, binary.LittleEndian, &addrs[i].Port)
		}
		return &MsgAddr{Addresses: addrs}, nil

	case CmdGetAddr:
		return &MsgGetAddr{}, nil

	case CmdGetHeaders:
		m := &MsgGetHeaders{}
		io.ReadFull(r, m.StartHash[:])
		io.ReadFull(r, m.StopHash[:])
		return m, nil

	case CmdHeaders:
		return &MsgHeaders{Headers: payload}, nil

	default:
		return nil, fmt.Errorf("network: unknown command %q", cmd)
	}
}

func decodeInvList(r *bytes.Reader, wrap func([]InvItem) Message) (Message, error) {
	var count uint32
	binary.Read(r, binary.LittleEndian, &count)
	if count > MaxInvItems {
		return nil, fmt.Errorf("network: inv count %d exceeds max %d", count, MaxInvItems)
	}
	items := make([]InvItem, count)
	for i := uint32(0); i < count; i++ {
		var t uint32
		binary.Read(r, binary.LittleEndian, &t)
		items[i].Type = InvType(t)
		io.ReadFull(r, items[i].Hash[:])
	}
	return wrap(items), nil
}

// --- string helpers ---

func writeString(buf *bytes.Buffer, s string) {
	binary.Write(buf, binary.LittleEndian, uint16(len(s)))
	buf.Write([]byte(s))
}

func readString(r *bytes.Reader) (string, error) {
	var length uint16
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	b := make([]byte, length)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return string(b), nil
}
