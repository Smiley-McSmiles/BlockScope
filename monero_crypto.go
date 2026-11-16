package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/sha3"
)

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var base58EncodedBlockSizes = []int{0, 2, 3, 5, 6, 7, 9, 10, 11}

const (
	mainnetAddressPrefix    = 18
	mainnetSubaddressPrefix = 42
	mainnetIntegratedPrefix = 19
	moneroAddressLength     = 69
	integratedAddressLength = 77
	moneroAddressChars      = 95
	integratedAddressChars  = 106
)

// Pedersen commitment base point H = toPoint(keccak(G)), as used by RingCT.
const moneroH = "8b655970153799af2aeadc9ff1add0ea6c7251d54154cfa92c173a0dd39c1f94"

var commitmentH = mustPoint(moneroH)

type moneroAddress struct {
	Subaddress bool
	Spend      *edwards25519.Point
	View       *edwards25519.Point
}

type ecdhInfo struct {
	Amount string `json:"amount"`
	Mask   string `json:"mask"`
}

type txOutput struct {
	Amount uint64 `json:"amount"`
	Target struct {
		Key       string `json:"key"`
		TaggedKey struct {
			Key string `json:"key"`
		} `json:"tagged_key"`
	} `json:"target"`
}

type txJSON struct {
	Vout          []txOutput `json:"vout"`
	Extra         []uint8    `json:"extra"`
	RctSignatures struct {
		Type     int        `json:"type"`
		TxnFee   uint64     `json:"txnFee"`
		EcdhInfo []ecdhInfo `json:"ecdhInfo"`
		OutPk    []string   `json:"outPk"`
	} `json:"rct_signatures"`
}

func (o txOutput) keyBytes() ([]byte, error) {
	k := o.Target.Key
	if k == "" {
		k = o.Target.TaggedKey.Key
	}
	return hex.DecodeString(k)
}

func mustPoint(h string) *edwards25519.Point {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	p, err := edwards25519.NewIdentityPoint().SetBytes(b)
	if err != nil {
		panic(err)
	}
	return p
}

func keccak256(parts ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

// hashToScalar is Monero's Hs: keccak256 reduced modulo the group order.
func hashToScalar(parts ...[]byte) *edwards25519.Scalar {
	wide := make([]byte, 64)
	copy(wide, keccak256(parts...))
	s, _ := edwards25519.NewScalar().SetUniformBytes(wide)
	return s
}

func varintBytes(n int) []byte {
	var b [binary.MaxVarintLen64]byte
	k := binary.PutUvarint(b[:], uint64(n))
	return b[:k]
}

func parseScalar(h string) (*edwards25519.Scalar, error) {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != 32 {
		return nil, errors.New("expected 32 bytes of hex")
	}
	s, err := edwards25519.NewScalar().SetCanonicalBytes(b)
	if err != nil {
		return nil, errors.New("not a valid private key")
	}
	return s, nil
}

func parsePointBytes(b []byte) (*edwards25519.Point, error) {
	return edwards25519.NewIdentityPoint().SetBytes(b)
}

// deriveSecret computes Monero's key derivation 8 * scalar * point.
// For a receiver it is 8*a*R (view key and tx pubkey), for a sender 8*r*A (tx key and recipient view key).
func deriveSecret(scalar *edwards25519.Scalar, point *edwards25519.Point) []byte {
	p := edwards25519.NewIdentityPoint().ScalarMult(scalar, point)
	return edwards25519.NewIdentityPoint().MultByCofactor(p).Bytes()
}

func sharedSecret(derivation []byte, index int) *edwards25519.Scalar {
	return hashToScalar(derivation, varintBytes(index))
}

// outputMatches reports whether the output key equals Hs(D||i)*G + B.
func outputMatches(derivation []byte, index int, outKey []byte, spend *edwards25519.Point) bool {
	s := sharedSecret(derivation, index)
	p := edwards25519.NewIdentityPoint().ScalarBaseMult(s)
	p.Add(p, spend)
	return bytes.Equal(p.Bytes(), outKey)
}

func edwardsBaseMult(s *edwards25519.Scalar) []byte {
	return edwards25519.NewIdentityPoint().ScalarBaseMult(s).Bytes()
}

func edwardsMult(s *edwards25519.Scalar, p *edwards25519.Point) []byte {
	return edwards25519.NewIdentityPoint().ScalarMult(s, p).Bytes()
}

func base58Decode(s string) ([]byte, error) {
	var out []byte
	for i := 0; i < len(s); i += 11 {
		end := i + 11
		if end > len(s) {
			end = len(s)
		}
		block := s[i:end]
		size := -1
		for idx, n := range base58EncodedBlockSizes {
			if n == len(block) {
				size = idx
				break
			}
		}
		if size < 0 {
			return nil, errors.New("invalid base58 block length")
		}
		num := new(big.Int)
		for _, c := range block {
			d := strings.IndexRune(base58Alphabet, c)
			if d < 0 {
				return nil, errors.New("invalid base58 character")
			}
			num.Mul(num, big.NewInt(58))
			num.Add(num, big.NewInt(int64(d)))
		}
		if num.BitLen() > 8*size {
			return nil, errors.New("invalid base58 block value")
		}
		buf := make([]byte, size)
		num.FillBytes(buf)
		out = append(out, buf...)
	}
	return out, nil
}

// parseMoneroAddress decodes a mainnet standard or subaddress and validates its checksum.
func parseMoneroAddress(s string) (*moneroAddress, error) {
	s = strings.TrimSpace(s)
	switch len(s) {
	case moneroAddressChars, integratedAddressChars:
	default:
		return nil, fmt.Errorf("a Monero address is %d characters (integrated addresses are %d), but this one is %d. Check that it was copied in full", moneroAddressChars, integratedAddressChars, len(s))
	}
	for _, c := range s {
		if c == ' ' || c == '\t' {
			return nil, errors.New("the address contains a space. Remove it and try again")
		}
		if !strings.ContainsRune(base58Alphabet, c) {
			return nil, fmt.Errorf("character %q is not allowed in a Monero address (0, O, I and l are never used)", c)
		}
	}
	raw, err := base58Decode(s)
	if err != nil || (len(raw) != moneroAddressLength && len(raw) != integratedAddressLength) {
		return nil, errors.New("could not decode the Monero address")
	}
	body, checksum := raw[:len(raw)-4], raw[len(raw)-4:]
	if !bytes.Equal(keccak256(body)[:4], checksum) {
		return nil, errors.New("checksum failed: the address has a typo or was copied incorrectly")
	}

	addr := &moneroAddress{}
	switch {
	case raw[0] == mainnetAddressPrefix && len(raw) == moneroAddressLength:
	case raw[0] == mainnetSubaddressPrefix && len(raw) == moneroAddressLength:
		addr.Subaddress = true
	case raw[0] == mainnetIntegratedPrefix && len(raw) == integratedAddressLength:
	default:
		return nil, errors.New("unsupported address type: only mainnet addresses, subaddresses and integrated addresses are supported")
	}

	// Integrated addresses carry the same spend and view keys as the standard
	// address, followed by a payment ID. The payment ID doesn't affect which
	// outputs belong to the address, so it's ignored.
	if addr.Spend, err = parsePointBytes(raw[1:33]); err != nil {
		return nil, errors.New("invalid spend key in address")
	}
	if addr.View, err = parsePointBytes(raw[33:65]); err != nil {
		return nil, errors.New("invalid view key in address")
	}
	return addr, nil
}

// parseTxPubKeys reads the transaction public key and additional public keys from tx extra.
func parseTxPubKeys(extra []byte) (mainKey []byte, additional [][]byte) {
	for i := 0; i < len(extra); {
		tag := extra[i]
		i++
		switch tag {
		case 0x00:
			continue
		case 0x01:
			if i+32 > len(extra) {
				return
			}
			mainKey = extra[i : i+32]
			i += 32
		case 0x02:
			n, k := binary.Uvarint(extra[i:])
			if k <= 0 {
				return
			}
			i += k + int(n)
		case 0x04:
			n, k := binary.Uvarint(extra[i:])
			if k <= 0 {
				return
			}
			i += k
			for j := uint64(0); j < n; j++ {
				if i+32 > len(extra) {
					return
				}
				additional = append(additional, extra[i:i+32])
				i += 32
			}
		default:
			return
		}
	}
	return
}

// decryptAmount recovers an output amount from its ECDH info and checks it against the
// Pedersen commitment C = mask*G + amount*H. Returns the amount and whether the commitment matched.
func decryptAmount(shared *edwards25519.Scalar, info ecdhInfo, commitment []byte) (uint64, bool, error) {
	encAmount, err := hex.DecodeString(info.Amount)
	if err != nil {
		return 0, false, errors.New("invalid ecdh amount")
	}

	var amount uint64
	var mask *edwards25519.Scalar

	if len(encAmount) == 8 {
		// RingCT v2 scheme (Bulletproof2 and later).
		factor := keccak256([]byte("amount"), shared.Bytes())
		var dec [8]byte
		for i := 0; i < 8; i++ {
			dec[i] = encAmount[i] ^ factor[i]
		}
		amount = binary.LittleEndian.Uint64(dec[:])
		mask = hashToScalar([]byte("commitment_mask"), shared.Bytes())
	} else if len(encAmount) == 32 {
		// Original RingCT scheme: amount and mask are additively blinded by two hashes.
		s1 := hashToScalar(shared.Bytes())
		s2 := hashToScalar(s1.Bytes())
		encScalar, err := edwards25519.NewScalar().SetCanonicalBytes(encAmount)
		if err != nil {
			return 0, false, errors.New("invalid ecdh amount")
		}
		encScalar.Subtract(encScalar, s2)
		res := encScalar.Bytes()
		if !bytes.Equal(res[8:], make([]byte, 24)) {
			return 0, false, errors.New("amount out of range")
		}
		amount = binary.LittleEndian.Uint64(res[:8])

		encMask, err := hex.DecodeString(info.Mask)
		if err != nil || len(encMask) != 32 {
			return amount, false, nil
		}
		mScalar, err := edwards25519.NewScalar().SetCanonicalBytes(encMask)
		if err != nil {
			return amount, false, nil
		}
		mask = mScalar.Subtract(mScalar, s1)
	} else {
		return 0, false, errors.New("unsupported ecdh amount size")
	}

	if len(commitment) != 32 || mask == nil {
		return amount, false, nil
	}
	amountBytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(amountBytes, amount)
	amountScalar, err := edwards25519.NewScalar().SetCanonicalBytes(amountBytes)
	if err != nil {
		return amount, false, nil
	}
	c := edwards25519.NewIdentityPoint().ScalarBaseMult(mask)
	c.Add(c, edwards25519.NewIdentityPoint().ScalarMult(amountScalar, commitmentH))
	return amount, bytes.Equal(c.Bytes(), commitment), nil
}
