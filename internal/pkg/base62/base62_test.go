package base62

import (
	"testing"
)

func TestBase62EncodeDecode(t *testing.T) {
	testCases := []uint64{
		0,
		1,
		61,
		62,
		1000,
		123456789,
		9876543210,
		18446744073709551615,
	}

	for _, tc := range testCases {
		encoded := Base62Encode(tc)
		decoded, err := Base62Decode(encoded)
		if err != nil {
			t.Fatalf("Base62Decode failed for %d (%s): %v", tc, encoded, err)
		}
		if decoded != tc {
			t.Fatalf("Expected %d, got %d for encoded string %s", tc, decoded, encoded)
		}
	}
}
