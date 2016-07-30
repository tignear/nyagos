package lua

import (
	"unsafe"
)

func CGoBytes(p, length uintptr) []byte {
	if length <= 0 || p == 0 {
		return []byte{}
	}
	buffer := make([]byte, length)
	for i := uintptr(0); i < length; i++ {
		buffer[i] = *(*byte)(unsafe.Pointer(p))
		p++
	}
	return buffer
}

func CGoStringN(p, length uintptr) string {
	if length <= 0 || p == 0 {
		return ""
	}
	return string(CGoBytes(p, length))
}

func CGoStringZ(p uintptr) string {
	result := make([]byte, 0)
	for {
		c := *(*byte)(unsafe.Pointer(p))
		if c == 0 {
			break
		}
		result = append(result, c)
		p++
	}
	return string(result)
}