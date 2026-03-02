package crypto

import (
	"errors"
	"strings"
)

// bech32mConst is the Bech32m checksum constant (BIP-350).
const bech32mConst = 0x2bc830a3

// charset is the Bech32 character set.
const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// charsetRev maps charset characters to their 5-bit values. -1 = invalid.
var charsetRev [128]int8

func init() {
	for i := range charsetRev {
		charsetRev[i] = -1
	}
	for i, c := range charset {
		charsetRev[c] = int8(i)
	}
}

// polymod computes the Bech32 BCH checksum polynomial.
func polymod(values []int) int {
	gen := [5]int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := 1
	for _, v := range values {
		b := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ v
		for i := 0; i < 5; i++ {
			if (b>>uint(i))&1 != 0 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

// hrpExpand expands the HRP for checksum computation.
func hrpExpand(hrp string) []int {
	ret := make([]int, 0, len(hrp)*2+1)
	for _, c := range hrp {
		ret = append(ret, int(c>>5))
	}
	ret = append(ret, 0)
	for _, c := range hrp {
		ret = append(ret, int(c&31))
	}
	return ret
}

// createChecksum computes the 6-value Bech32m checksum.
func createChecksum(hrp string, data []int) []int {
	values := append(hrpExpand(hrp), data...)
	values = append(values, 0, 0, 0, 0, 0, 0)
	mod := polymod(values) ^ bech32mConst
	ret := make([]int, 6)
	for i := 0; i < 6; i++ {
		ret[i] = (mod >> uint(5*(5-i))) & 31
	}
	return ret
}

// verifyChecksum returns true if the data (including checksum) is valid for the given HRP.
func verifyChecksum(hrp string, data []int) bool {
	return polymod(append(hrpExpand(hrp), data...)) == bech32mConst
}

// Bech32mEncode encodes a human-readable part and 5-bit data values into a Bech32m string.
func Bech32mEncode(hrp string, data []byte) (string, error) {
	if len(hrp) == 0 {
		return "", errors.New("bech32m: empty HRP")
	}
	for _, c := range hrp {
		if c < 33 || c > 126 {
			return "", errors.New("bech32m: invalid HRP character")
		}
	}

	// Convert data to int slice for checksum computation.
	dataInts := make([]int, len(data))
	for i, b := range data {
		if b > 31 {
			return "", errors.New("bech32m: data value exceeds 5 bits")
		}
		dataInts[i] = int(b)
	}

	checksum := createChecksum(hrp, dataInts)

	var sb strings.Builder
	sb.Grow(len(hrp) + 1 + len(data) + 6)
	sb.WriteString(strings.ToLower(hrp))
	sb.WriteByte('1')
	for _, d := range dataInts {
		sb.WriteByte(charset[d])
	}
	for _, d := range checksum {
		sb.WriteByte(charset[d])
	}
	return sb.String(), nil
}

// Bech32mDecode decodes a Bech32m string into its HRP and 5-bit data values.
func Bech32mDecode(s string) (string, []byte, error) {
	if len(s) > 90 {
		return "", nil, errors.New("bech32m: string too long")
	}

	// Check for mixed case.
	hasLower, hasUpper := false, false
	for _, c := range s {
		if c >= 'a' && c <= 'z' {
			hasLower = true
		}
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
	}
	if hasLower && hasUpper {
		return "", nil, errors.New("bech32m: mixed case")
	}
	s = strings.ToLower(s)

	// Find the separator '1'.
	sepIdx := strings.LastIndex(s, "1")
	if sepIdx < 1 {
		return "", nil, errors.New("bech32m: missing separator")
	}
	if sepIdx+7 > len(s) {
		return "", nil, errors.New("bech32m: data part too short")
	}

	hrp := s[:sepIdx]
	dataPart := s[sepIdx+1:]

	// Decode data part.
	data := make([]int, len(dataPart))
	for i, c := range []byte(dataPart) {
		if c >= 128 || charsetRev[c] == -1 {
			return "", nil, errors.New("bech32m: invalid character")
		}
		data[i] = int(charsetRev[c])
	}

	if !verifyChecksum(hrp, data) {
		return "", nil, errors.New("bech32m: invalid checksum")
	}

	// Strip checksum (last 6 values).
	data = data[:len(data)-6]
	result := make([]byte, len(data))
	for i, d := range data {
		result[i] = byte(d)
	}
	return hrp, result, nil
}
