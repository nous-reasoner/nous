package vdf

import (
	"bytes"
	"testing"

	"github.com/nous-chain/nous/crypto"
)

// T=1000 as specified: exercises full pipeline while staying fast (~10ms).
const testT = 1000

func TestEvaluateAndVerify(t *testing.T) {
	params := NewParams(testT)

	prevHash := crypto.Sha256([]byte("previous block"))
	_, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	input := MakeInput(prevHash, pub)
	output, err := Evaluate(params, input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if !Verify(params, input, output) {
		t.Fatal("valid VDF output should pass verification")
	}
}

func TestEvaluateAndVerifyRawInput(t *testing.T) {
	params := NewParams(testT)
	input := crypto.Sha256([]byte("raw input test"))

	output, err := Evaluate(params, input[:])
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if !Verify(params, input[:], output) {
		t.Fatal("valid VDF output should pass verification")
	}
}

func TestVerifyRejectsTamperedY(t *testing.T) {
	params := NewParams(testT)
	input := crypto.Sha256([]byte("tamper Y"))

	output, err := Evaluate(params, input[:])
	if err != nil {
		t.Fatal(err)
	}

	tampered := &Output{
		Y:     make([]byte, len(output.Y)),
		Proof: make([]byte, len(output.Proof)),
	}
	copy(tampered.Y, output.Y)
	copy(tampered.Proof, output.Proof)
	// Flip a bit in Y
	tampered.Y[len(tampered.Y)/2] ^= 0x01

	if Verify(params, input[:], tampered) {
		t.Fatal("tampered Y should not verify")
	}
}

func TestVerifyRejectsTamperedProof(t *testing.T) {
	params := NewParams(testT)
	input := crypto.Sha256([]byte("tamper proof"))

	output, err := Evaluate(params, input[:])
	if err != nil {
		t.Fatal(err)
	}

	tampered := &Output{
		Y:     make([]byte, len(output.Y)),
		Proof: make([]byte, len(output.Proof)),
	}
	copy(tampered.Y, output.Y)
	copy(tampered.Proof, output.Proof)
	tampered.Proof[len(tampered.Proof)/2] ^= 0x01

	if Verify(params, input[:], tampered) {
		t.Fatal("tampered proof should not verify")
	}
}

func TestVerifyRejectsWrongInput(t *testing.T) {
	params := NewParams(testT)
	input1 := crypto.Sha256([]byte("input 1"))
	input2 := crypto.Sha256([]byte("input 2"))

	output, err := Evaluate(params, input1[:])
	if err != nil {
		t.Fatal(err)
	}

	if Verify(params, input2[:], output) {
		t.Fatal("wrong input should not verify")
	}
}

func TestVerifyRejectsWrongT(t *testing.T) {
	params := NewParams(testT)
	input := crypto.Sha256([]byte("wrong T"))

	output, err := Evaluate(params, input[:])
	if err != nil {
		t.Fatal(err)
	}

	// Verify with different T should fail
	wrongParams := NewParams(testT + 1)
	if Verify(wrongParams, input[:], output) {
		t.Fatal("wrong T should not verify")
	}
}

func TestDeterministic(t *testing.T) {
	params := NewParams(testT)
	input := crypto.Sha256([]byte("deterministic"))

	output1, err := Evaluate(params, input[:])
	if err != nil {
		t.Fatal(err)
	}

	output2, err := Evaluate(params, input[:])
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(output1.Y, output2.Y) {
		t.Fatal("same input should produce identical Y")
	}
	if !bytes.Equal(output1.Proof, output2.Proof) {
		t.Fatal("same input should produce identical Proof")
	}
}

func TestDifferentInputsDifferentOutputs(t *testing.T) {
	params := NewParams(testT)
	inputA := crypto.Sha256([]byte("input A"))
	inputB := crypto.Sha256([]byte("input B"))

	outputA, _ := Evaluate(params, inputA[:])
	outputB, _ := Evaluate(params, inputB[:])

	if bytes.Equal(outputA.Y, outputB.Y) {
		t.Fatal("different inputs should produce different Y values")
	}
}

func TestMakeInputPerMinerUniqueness(t *testing.T) {
	prevHash := crypto.Sha256([]byte("block hash"))
	_, pub1, _ := crypto.GenerateKeyPair()
	_, pub2, _ := crypto.GenerateKeyPair()

	input1 := MakeInput(prevHash, pub1)
	input2 := MakeInput(prevHash, pub2)

	if bytes.Equal(input1, input2) {
		t.Fatal("different pubkeys must produce different VDF inputs")
	}

	// Same miner, same block → deterministic
	input1b := MakeInput(prevHash, pub1)
	if !bytes.Equal(input1, input1b) {
		t.Fatal("same pubkey + same block should produce identical input")
	}
}

func TestOutputHash(t *testing.T) {
	params := NewParams(testT)
	input := crypto.Sha256([]byte("hash test"))

	output, _ := Evaluate(params, input[:])

	h1 := output.OutputHash()
	h2 := output.OutputHash()
	if h1.IsZero() {
		t.Fatal("output hash should not be zero")
	}
	if h1 != h2 {
		t.Fatal("output hash should be deterministic")
	}

	ph := output.ProofHash()
	if ph.IsZero() {
		t.Fatal("proof hash should not be zero")
	}
	if h1 == ph {
		t.Fatal("output hash and proof hash should differ")
	}
}

func TestOutputByteLengths(t *testing.T) {
	params := NewParams(testT)
	input := crypto.Sha256([]byte("length check"))

	output, _ := Evaluate(params, input[:])

	modLen := (params.N.BitLen() + 7) / 8
	if len(output.Y) != modLen {
		t.Fatalf("Y should be %d bytes, got %d", modLen, len(output.Y))
	}
	if len(output.Proof) != modLen {
		t.Fatalf("Proof should be %d bytes, got %d", modLen, len(output.Proof))
	}
}

func TestEdgeCaseTEquals1(t *testing.T) {
	params := NewParams(1)
	input := crypto.Sha256([]byte("T=1"))

	output, err := Evaluate(params, input[:])
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(params, input[:], output) {
		t.Fatal("T=1 should still produce a valid VDF")
	}
}

func TestEdgeCaseTEquals2(t *testing.T) {
	params := NewParams(2)
	input := crypto.Sha256([]byte("T=2"))

	output, err := Evaluate(params, input[:])
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(params, input[:], output) {
		t.Fatal("T=2 should produce a valid VDF")
	}
}

func TestEvaluateRejectsInvalidParams(t *testing.T) {
	input := crypto.Sha256([]byte("bad params"))

	_, err := Evaluate(&Params{N: nil, T: 100}, input[:])
	if err == nil {
		t.Fatal("nil modulus should be rejected")
	}

	_, err = Evaluate(&Params{N: defaultModulus, T: 0}, input[:])
	if err == nil {
		t.Fatal("T=0 should be rejected")
	}
}

func TestVerifyRejectsNilOutput(t *testing.T) {
	params := NewParams(testT)
	input := crypto.Sha256([]byte("nil"))

	if Verify(params, input[:], nil) {
		t.Fatal("nil output should not verify")
	}

	if Verify(params, input[:], &Output{}) {
		t.Fatal("empty output should not verify")
	}
}
