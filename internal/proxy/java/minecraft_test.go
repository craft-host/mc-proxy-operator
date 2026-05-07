package java

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func buildHandshakePacket(protocolVersion int32, serverAddress string, serverPort uint16, nextState int32) []byte {
	var payload []byte
	payload = appendVarInt(payload, HandshakePacketID)
	payload = appendVarInt(payload, protocolVersion)
	payload = appendMCString(payload, serverAddress)

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, serverPort)
	payload = append(payload, portBytes...)

	payload = appendVarInt(payload, nextState)

	var packet []byte
	packet = appendVarInt(packet, int32(len(payload)))
	packet = append(packet, payload...)

	return packet
}

func appendVarInt(buf []byte, value int32) []byte {
	u := uint32(value)
	for {
		b := byte(u & 0x7F)
		u >>= 7
		if u != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if u == 0 {
			break
		}
	}
	return buf
}

func appendMCString(buf []byte, s string) []byte {
	buf = appendVarInt(buf, int32(len(s)))
	buf = append(buf, []byte(s)...)
	return buf
}

func TestReadHandshake_ValidLogin(t *testing.T) {
	packet := buildHandshakePacket(765, "jugador1.example.com", 25565, StateLogin)

	hs, rawBytes, err := ReadHandshake(bytes.NewReader(packet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hs.ProtocolVersion != 765 {
		t.Fatalf("expected protocol 765, got %d", hs.ProtocolVersion)
	}
	if hs.ServerAddress != "jugador1.example.com" {
		t.Fatalf("expected address 'jugador1.example.com', got '%s'", hs.ServerAddress)
	}
	if hs.ServerPort != 25565 {
		t.Fatalf("expected port 25565, got %d", hs.ServerPort)
	}
	if hs.NextState != StateLogin {
		t.Fatalf("expected state %d, got %d", StateLogin, hs.NextState)
	}

	if !bytes.Equal(rawBytes, packet) {
		t.Fatal("rawBytes do not match original packet")
	}
}

func TestReadHandshake_ValidStatus(t *testing.T) {
	packet := buildHandshakePacket(765, "test.example.com", 25565, StateStatus)

	hs, _, err := ReadHandshake(bytes.NewReader(packet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hs.NextState != StateStatus {
		t.Fatalf("expected state %d, got %d", StateStatus, hs.NextState)
	}
}

func TestReadHandshake_ForgeClient(t *testing.T) {
	forgeAddr := "jugador1.example.com\x00FML\x00"
	packet := buildHandshakePacket(765, forgeAddr, 25565, StateLogin)

	hs, _, err := ReadHandshake(bytes.NewReader(packet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hs.ServerAddress != "jugador1.example.com" {
		t.Fatalf("expected clean address 'jugador1.example.com', got '%s'", hs.ServerAddress)
	}
}

func TestReadHandshake_FML2Client(t *testing.T) {
	fml2Addr := "jugador1.example.com\x00FML2\x00"
	packet := buildHandshakePacket(765, fml2Addr, 25565, StateLogin)

	hs, _, err := ReadHandshake(bytes.NewReader(packet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hs.ServerAddress != "jugador1.example.com" {
		t.Fatalf("expected clean address, got '%s'", hs.ServerAddress)
	}
}

func TestReadHandshake_CaseInsensitive(t *testing.T) {
	packet := buildHandshakePacket(765, "Jugador1.EXAMPLE.Com", 25565, StateLogin)

	hs, _, err := ReadHandshake(bytes.NewReader(packet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hs.ServerAddress != "jugador1.example.com" {
		t.Fatalf("expected lowercase address, got '%s'", hs.ServerAddress)
	}
}

func TestReadHandshake_InvalidPacketID(t *testing.T) {
	var payload []byte
	payload = appendVarInt(payload, 0x01) // wrong packet ID
	payload = appendVarInt(payload, 765)
	payload = appendMCString(payload, "test.com")
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, 25565)
	payload = append(payload, portBytes...)
	payload = appendVarInt(payload, StateLogin)

	var packet []byte
	packet = appendVarInt(packet, int32(len(payload)))
	packet = append(packet, payload...)

	_, _, err := ReadHandshake(bytes.NewReader(packet))
	if err == nil {
		t.Fatal("expected error for invalid packet ID")
	}
}

func TestReadHandshake_TruncatedPayload(t *testing.T) {
	packet := buildHandshakePacket(765, "test.com", 25565, StateLogin)

	_, _, err := ReadHandshake(bytes.NewReader(packet[:5]))
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestReadHandshake_OversizedPacket(t *testing.T) {
	var payload []byte
	payload = appendVarInt(payload, HandshakePacketID)
	payload = appendVarInt(payload, 765)
	longAddr := ""
	for i := 0; i < 600; i++ {
		longAddr += "a"
	}
	payload = appendMCString(payload, longAddr)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, 25565)
	payload = append(payload, portBytes...)
	payload = appendVarInt(payload, StateLogin)

	var packet []byte
	packet = appendVarInt(packet, int32(len(payload)))
	packet = append(packet, payload...)

	_, _, err := ReadHandshake(bytes.NewReader(packet))
	if err == nil {
		t.Fatal("expected error for oversized packet")
	}
}

func TestReadHandshake_RawBytesAreCorrect(t *testing.T) {
	packet := buildHandshakePacket(765, "test.example.com", 25565, StateLogin)

	_, rawBytes, err := ReadHandshake(bytes.NewReader(packet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(rawBytes, packet) {
		t.Fatalf("rawBytes mismatch:\nexpected: %x\nactual:   %x", packet, rawBytes)
	}
}
