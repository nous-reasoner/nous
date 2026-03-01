package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/nous-chain/nous/sat"
)

var (
	varCounts = []int{30, 50, 80, 100, 150, 200}
	ratios    = []float64{2.5, 3.0, 3.2, 3.5}
)

const enumTimeout = 30 * time.Second

func main() {
	fmt.Println("=== 3-SAT Benchmark (WalkSAT Enumeration, 30s per problem) ===")
	fmt.Println()
	fmt.Printf("%-5s | %-5s | %-7s | %-10s | %-10s | %s\n",
		"N", "R", "Clauses", "Solutions", "Sol/sec", "Verified")
	fmt.Println(strings.Repeat("-", 65))

	for _, n := range varCounts {
		for _, r := range ratios {
			seed := makeSeed(n, r, 0)
			m := int(math.Ceil(float64(n) * r))
			f := sat.GenerateFormula(seed, n, r)

			start := time.Now()
			count, solutions := sat.WalkSATEnumerate(f, n, enumTimeout)
			elapsed := time.Since(start)

			// Verify first few solutions.
			verified := "n/a"
			if count > 0 {
				allOK := true
				for _, sol := range solutions {
					if !sat.Verify(f, sol) {
						allOK = false
						break
					}
				}
				if allOK {
					verified = "YES"
				} else {
					verified = "NO"
				}
			}

			rate := float64(count) / elapsed.Seconds()

			fmt.Printf("%-5d | %-5.1f | %-7d | %-10d | %-10.1f | %s\n",
				n, r, m, count, rate, verified)
		}
	}

	fmt.Println(strings.Repeat("-", 65))
	fmt.Println("Done.")
}

func makeSeed(n int, r float64, idx int) [32]byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint32(buf[0:4], uint32(n))
	binary.BigEndian.PutUint64(buf[4:12], math.Float64bits(r))
	binary.BigEndian.PutUint32(buf[12:16], uint32(idx))
	return sha256.Sum256(buf)
}
