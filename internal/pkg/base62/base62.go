package base62

import (
	"errors"
	"strings"
)

const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const base = uint64(len(alphabet))

// Base62Encode converts a uint64 number to Base62 string representation
func Base62Encode(num uint64) string {
	if num == 0 {
		return string(alphabet[0])
	}

	var sb strings.Builder
	for num > 0 {
		rem := num % base
		sb.WriteByte(alphabet[rem])
		num = num / base
	}

	// Reverse the string
	runes := []rune(sb.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// Base62Decode converts a Base62 string back to uint64
func Base62Decode(str string) (uint64, error) {
	var num uint64
	for _, char := range str {
		idx := strings.IndexRune(alphabet, char)
		if idx == -1 {
			return 0, errors.New("invalid base62 character")
		}
		num = num*base + uint64(idx)
	}
	return num, nil
}
