package java

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	MaxVarIntBytes         = 5
	MaxHandshakeSize       = 512
	MaxServerAddressLength = 255
	HandshakePacketID      = 0x00
	StateStatus            = 1
	StateLogin             = 2
)

type Handshake struct {
	ProtocolVersion int32
	ServerAddress   string
	ServerPort      uint16
	NextState       int32
}

func ReadHandshake(reader io.Reader) (*Handshake, []byte, error) {
	packetLength, lengthBytes, err := readVarIntRaw(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo packet length: %w", err)
	}
	if packetLength <= 0 || packetLength > MaxHandshakeSize {
		return nil, nil, fmt.Errorf("packet length inválido: %d", packetLength)
	}

	payload := make([]byte, packetLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, nil, fmt.Errorf("leyendo payload: %w", err)
	}

	rawBytes := append(lengthBytes, payload...)

	offset := 0

	packetID, n, err := readVarIntFromBytes(payload, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo packet ID: %w", err)
	}
	offset += n
	if packetID != HandshakePacketID {
		return nil, nil, fmt.Errorf("packet ID inesperado: 0x%02X", packetID)
	}

	protocolVersion, n, err := readVarIntFromBytes(payload, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo protocol version: %w", err)
	}
	offset += n

	serverAddress, n, err := readStringFromBytes(payload, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo server address: %w", err)
	}
	offset += n

	if offset+2 > len(payload) {
		return nil, nil, errors.New("payload truncado en server port")
	}
	serverPort := binary.BigEndian.Uint16(payload[offset : offset+2])
	offset += 2

	nextState, _, err := readVarIntFromBytes(payload, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo next state: %w", err)
	}

	return &Handshake{
		ProtocolVersion: protocolVersion,
		ServerAddress:   cleanServerAddress(serverAddress),
		ServerPort:      serverPort,
		NextState:       nextState,
	}, rawBytes, nil
}

func cleanServerAddress(address string) string {
	if idx := strings.IndexByte(address, 0x00); idx != -1 {
		address = address[:idx]
	}
	return strings.ToLower(strings.TrimSpace(address))
}

func readVarIntRaw(reader io.Reader) (int32, []byte, error) {
	var result int32
	var numRead uint
	var rawBytes []byte
	buf := make([]byte, 1)

	for {
		if _, err := io.ReadFull(reader, buf); err != nil {
			return 0, nil, err
		}
		rawBytes = append(rawBytes, buf[0])
		result |= int32(buf[0]&0x7F) << (7 * numRead)
		numRead++
		if numRead > MaxVarIntBytes {
			return 0, nil, errors.New("VarInt demasiado grande")
		}
		if buf[0]&0x80 == 0 {
			break
		}
	}
	return result, rawBytes, nil
}

func readVarIntFromBytes(data []byte, offset int) (int32, int, error) {
	var result int32
	var numRead int

	for {
		if offset+numRead >= len(data) {
			return 0, 0, errors.New("VarInt truncado")
		}
		b := data[offset+numRead]
		result |= int32(b&0x7F) << (7 * numRead)
		numRead++
		if numRead > MaxVarIntBytes {
			return 0, 0, errors.New("VarInt demasiado grande")
		}
		if b&0x80 == 0 {
			break
		}
	}
	return result, numRead, nil
}

func readStringFromBytes(data []byte, offset int) (string, int, error) {
	strLen, n, err := readVarIntFromBytes(data, offset)
	if err != nil {
		return "", 0, err
	}
	if strLen < 0 || strLen > MaxServerAddressLength {
		return "", 0, fmt.Errorf("longitud de string inválida: %d", strLen)
	}
	start := offset + n
	end := start + int(strLen)
	if end > len(data) {
		return "", 0, errors.New("string truncado")
	}
	return string(data[start:end]), n + int(strLen), nil
}
