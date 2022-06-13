package bit_utils

import (
	"encoding/binary"
)

func GetBytesFromUInt32(len uint32) []byte {
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, len)
	return bs
}

func GetUInt32FromBytes(lens []byte) uint32 {
	return binary.LittleEndian.Uint32(lens)
}

func AddSize(data []byte) []byte {
	ld := uint32(len(data))
	return append(GetBytesFromUInt32(ld), data...)
}
