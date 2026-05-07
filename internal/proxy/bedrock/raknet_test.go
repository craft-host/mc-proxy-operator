package bedrock

import "testing"

func buildUnconnectedPing() []byte {
	packet := make([]byte, 33)
	packet[0] = IDUnconnectedPing

	for i := 1; i <= 8; i++ {
		packet[i] = 0x00
	}

	copy(packet[9:25], RakNetMagic)

	for i := 25; i < 33; i++ {
		packet[i] = 0xFF
	}

	return packet
}

func TestIsRakNetPacket_ValidPing(t *testing.T) {
	packet := buildUnconnectedPing()

	if !IsRakNetPacket(packet) {
		t.Fatal("expected valid RakNet ping to be detected")
	}
}

func TestIsRakNetPacket_InvalidMagic(t *testing.T) {
	packet := buildUnconnectedPing()

	for i := 9; i < 25; i++ {
		packet[i] = 0x00
	}

	if IsRakNetPacket(packet) {
		t.Fatal("expected invalid magic to be rejected")
	}
}

func TestIsRakNetPacket_TooShort(t *testing.T) {
	packet := make([]byte, 10)

	if IsRakNetPacket(packet) {
		t.Fatal("expected short packet to be rejected")
	}
}

func TestIsRakNetPacket_WrongPacketID(t *testing.T) {
	packet := buildUnconnectedPing()
	packet[0] = 0x05

	if IsRakNetPacket(packet) {
		t.Fatal("expected wrong packet ID to be rejected")
	}
}
