package bedrock

const (
	IDUnconnectedPing = 0x01

	IDUnconnectedPong = 0x1C
)

var RakNetMagic = []byte{
	0x00, 0xff, 0xff, 0x00,
	0xfe, 0xfe, 0xfe, 0xfe,
	0xfd, 0xfd, 0xfd, 0xfd,
	0x12, 0x34, 0x56, 0x78,
}

func IsRakNetPacket(data []byte) bool {
	if len(data) < 33 {
		return false
	}

	if data[0] == IDUnconnectedPing {
		return bytesEqual(data[9:25], RakNetMagic)
	}
	return false
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
