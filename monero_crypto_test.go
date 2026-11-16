package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"

	"filippo.io/edwards25519"
)

func randScalar(t *testing.T) *edwards25519.Scalar {
	t.Helper()
	var wide [64]byte
	if _, err := rand.Read(wide[:]); err != nil {
		t.Fatal(err)
	}
	s, err := edwards25519.NewScalar().SetUniformBytes(wide[:])
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func scalarFromUint64(v uint64) *edwards25519.Scalar {
	b := make([]byte, 32)
	binary.LittleEndian.PutUint64(b, v)
	s, _ := edwards25519.NewScalar().SetCanonicalBytes(b)
	return s
}

// A known mainnet subaddress, which exercises the subaddress prefix.
func TestParseDonationSubaddress(t *testing.T) {
	addr, err := parseMoneroAddress("892HHTyDg5mJm5eWJWZ8L1ZMYnnWExciQFFkpsgLh1DfVUXfUFj6z1X2jDD2ZRQLiwWYskeyNkrtpAHse4M3G29uBfiYgVL")
	if err != nil {
		t.Fatal(err)
	}
	if !addr.Subaddress {
		t.Fatal("expected a subaddress")
	}
}

func TestParseAddressErrorsExplainProblem(t *testing.T) {
	const good = "892HHTyDg5mJm5eWJWZ8L1ZMYnnWExciQFFkpsgLh1DfVUXfUFj6z1X2jDD2ZRQLiwWYskeyNkrtpAHse4M3G29uBfiYgVL"
	cases := map[string]string{
		"truncated":   good[:90],
		"too long":    good + good[:11],
		"bad char":    strings.Replace(good, "L", "0", 1),
		"inner space": good[:40] + " " + good[41:],
	}
	for name, in := range cases {
		_, err := parseMoneroAddress(in)
		if err == nil {
			t.Fatalf("%s: expected error", name)
		}
		t.Logf("%s: %v", name, err)
	}
	if _, err := parseMoneroAddress("  " + good + "\n"); err != nil {
		t.Fatalf("surrounding whitespace should be accepted: %v", err)
	}
}

// Integrated addresses (106 characters) wrap a standard address with a payment ID.
func TestIntegratedAddressUsesSameKeys(t *testing.T) {
	spend := pointBytes(randScalar(t))
	view := pointBytes(randScalar(t))
	paymentID := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	standard := encodeAddressForTest(mainnetAddressPrefix, spend, view)
	raw := append([]byte{mainnetIntegratedPrefix}, spend...)
	raw = append(raw, view...)
	raw = append(raw, paymentID...)
	raw = append(raw, keccak256(raw)[:4]...)
	integrated := base58EncodeForTest(raw)
	if len(integrated) != integratedAddressChars {
		t.Fatalf("integrated address is %d characters", len(integrated))
	}

	a, err := parseMoneroAddress(standard)
	if err != nil {
		t.Fatal(err)
	}
	b, err := parseMoneroAddress(integrated)
	if err != nil {
		t.Fatalf("integrated address rejected: %v", err)
	}
	if !bytes.Equal(a.Spend.Bytes(), b.Spend.Bytes()) || !bytes.Equal(a.View.Bytes(), b.View.Bytes()) {
		t.Fatal("integrated address decoded to different keys")
	}
}

func TestParseAddressRejectsBadChecksum(t *testing.T) {
	bad := "892HHTyDg5mJm5eWJWZ8L1ZMYnnWExciQFFkpsgLh1DfVUXfUFj6z1X2jDD2ZRQLiwWYskeyNkrtpAHse4M3G29uBfiYgVM"
	if _, err := parseMoneroAddress(bad); err == nil {
		t.Fatal("expected an error for a modified address")
	}
}

func TestOutputRoundTrip(t *testing.T) {
	r := randScalar(t)
	a := randScalar(t)
	b := randScalar(t)
	R := edwards25519.NewIdentityPoint().ScalarBaseMult(r)
	A := edwards25519.NewIdentityPoint().ScalarBaseMult(a)
	B := edwards25519.NewIdentityPoint().ScalarBaseMult(b)

	// Sender side: the derivation from the tx key and the recipient's view key.
	const index = 1
	senderD := deriveSecret(r, A)
	// Receiver side: the derivation from the view key and the tx public key.
	receiverD := deriveSecret(a, R)
	if !bytes.Equal(senderD, receiverD) {
		t.Fatal("sender and receiver derivations differ")
	}

	shared := sharedSecret(senderD, index)
	outKey := edwards25519.NewIdentityPoint().ScalarBaseMult(shared)
	outKey.Add(outKey, B)

	if !outputMatches(receiverD, index, outKey.Bytes(), B) {
		t.Fatal("receiver did not recognise its own output")
	}
	if outputMatches(receiverD, 0, outKey.Bytes(), B) {
		t.Fatal("output matched at the wrong index")
	}
	other := randScalar(t)
	if outputMatches(deriveSecret(other, R), index, outKey.Bytes(), B) {
		t.Fatal("output matched with an unrelated view key")
	}

	const amount = uint64(1234567890123)
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, amount)

	// RingCT v2 encoding: XOR the amount with keccak("amount"||shared), mask from "commitment_mask".
	factor := keccak256([]byte("amount"), shared.Bytes())
	enc := make([]byte, 8)
	for i := range enc {
		enc[i] = amountBytes[i] ^ factor[i]
	}
	mask := hashToScalar([]byte("commitment_mask"), shared.Bytes())
	commit := edwards25519.NewIdentityPoint().ScalarBaseMult(mask)
	commit.Add(commit, edwards25519.NewIdentityPoint().ScalarMult(scalarFromUint64(amount), commitmentH))

	info := ecdhInfo{Amount: hex.EncodeToString(enc)}
	got, verified, err := decryptAmount(sharedSecret(receiverD, index), info, commit.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got != amount || !verified {
		t.Fatalf("v2 amount: got %d verified=%v, want %d verified=true", got, verified, amount)
	}

	wrongCommit := edwards25519.NewIdentityPoint().ScalarBaseMult(mask)
	got, verified, err = decryptAmount(sharedSecret(receiverD, index), info, wrongCommit.Bytes())
	if err != nil || got != amount || verified {
		t.Fatalf("v2 wrong commitment: got %d verified=%v err=%v", got, verified, err)
	}
}

// Original RingCT encoding: amount and mask are blinded by additive hashes of the shared secret.
func TestAmountRoundTripV1(t *testing.T) {
	shared := randScalar(t)
	const amount = uint64(987654321)

	s1 := hashToScalar(shared.Bytes())
	s2 := hashToScalar(s1.Bytes())
	mask := randScalar(t)

	encAmount := edwards25519.NewScalar().Add(scalarFromUint64(amount), s2)
	encMask := edwards25519.NewScalar().Add(mask, s1)
	commit := edwards25519.NewIdentityPoint().ScalarBaseMult(mask)
	commit.Add(commit, edwards25519.NewIdentityPoint().ScalarMult(scalarFromUint64(amount), commitmentH))

	info := ecdhInfo{Amount: hex.EncodeToString(encAmount.Bytes()), Mask: hex.EncodeToString(encMask.Bytes())}
	got, verified, err := decryptAmount(shared, info, commit.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got != amount || !verified {
		t.Fatalf("v1 amount: got %d verified=%v, want %d verified=true", got, verified, amount)
	}
}

func TestParseTxPubKeys(t *testing.T) {
	main := bytes.Repeat([]byte{0x11}, 32)
	add1 := bytes.Repeat([]byte{0x22}, 32)
	add2 := bytes.Repeat([]byte{0x33}, 32)

	extra := []byte{0x00, 0x01}
	extra = append(extra, main...)
	extra = append(extra, 0x04, 0x02)
	extra = append(extra, add1...)
	extra = append(extra, add2...)

	gotMain, gotAdd := parseTxPubKeys(extra)
	if !bytes.Equal(gotMain, main) {
		t.Fatal("main tx public key not parsed")
	}
	if len(gotAdd) != 2 || !bytes.Equal(gotAdd[0], add1) || !bytes.Equal(gotAdd[1], add2) {
		t.Fatal("additional public keys not parsed")
	}
}
