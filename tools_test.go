package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"filippo.io/edwards25519"
	"github.com/cirocosta/go-monero/pkg/rpc"
	"github.com/cirocosta/go-monero/pkg/rpc/daemon"
)

func base58EncodeForTest(data []byte) string {
	var out []byte
	for i := 0; i < len(data); i += 8 {
		end := i + 8
		if end > len(data) {
			end = len(data)
		}
		block := data[i:end]
		chars := make([]byte, base58EncodedBlockSizes[len(block)])
		num := new(big.Int).SetBytes(block)
		mod := new(big.Int)
		for j := len(chars) - 1; j >= 0; j-- {
			num.DivMod(num, big.NewInt(58), mod)
			chars[j] = base58Alphabet[mod.Int64()]
		}
		out = append(out, chars...)
	}
	return string(out)
}

func encodeAddressForTest(prefix byte, spend, view []byte) string {
	raw := append([]byte{prefix}, spend...)
	raw = append(raw, view...)
	raw = append(raw, keccak256(raw)[:4]...)
	return base58EncodeForTest(raw)
}

func pointBytes(s *edwards25519.Scalar) []byte {
	return edwards25519.NewIdentityPoint().ScalarBaseMult(s).Bytes()
}

func intsOf(b []byte) []int {
	out := make([]int, len(b))
	for i, v := range b {
		out[i] = int(v)
	}
	return out
}

// buildTestTx creates a RingCT transaction with one output to the recipient and one unrelated output.
func buildTestTx(t *testing.T, r, a, b *edwards25519.Scalar, amount uint64) (txJSONStr string, ourAddr string) {
	t.Helper()
	R := pointBytes(r)
	A := edwards25519.NewIdentityPoint().ScalarBaseMult(a)
	B := edwards25519.NewIdentityPoint().ScalarBaseMult(b)

	// Output 0: unrelated random key.
	otherKey := pointBytes(randScalar(t))

	// Output 1: paid to us.
	const index = 1
	derivation := deriveSecret(r, A)
	shared := sharedSecret(derivation, index)
	ourKey := edwards25519.NewIdentityPoint().ScalarBaseMult(shared)
	ourKey.Add(ourKey, B)

	amountBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		amountBytes[i] = byte(amount >> (8 * i))
	}
	factor := keccak256([]byte("amount"), shared.Bytes())
	enc := make([]byte, 8)
	for i := range enc {
		enc[i] = amountBytes[i] ^ factor[i]
	}
	mask := hashToScalar([]byte("commitment_mask"), shared.Bytes())
	commit := edwards25519.NewIdentityPoint().ScalarBaseMult(mask)
	commit.Add(commit, edwards25519.NewIdentityPoint().ScalarMult(scalarFromUint64(amount), commitmentH))

	extra := append([]byte{0x01}, R...)

	tx := map[string]interface{}{
		"version": 2,
		"vout": []map[string]interface{}{
			{"amount": 0, "target": map[string]interface{}{"key": hex.EncodeToString(otherKey)}},
			{"amount": 0, "target": map[string]interface{}{"tagged_key": map[string]interface{}{"key": hex.EncodeToString(ourKey.Bytes()), "view_tag": "00"}}},
		},
		"extra": intsOf(extra),
		"rct_signatures": map[string]interface{}{
			"type": 6,
			"ecdhInfo": []map[string]string{
				{"amount": hex.EncodeToString(bytes.Repeat([]byte{0x5a}, 8))},
				{"amount": hex.EncodeToString(enc)},
			},
			"outPk": []string{hex.EncodeToString(pointBytes(randScalar(t))), hex.EncodeToString(commit.Bytes())},
		},
	}
	raw, err := json.Marshal(tx)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), encodeAddressForTest(mainnetAddressPrefix, B.Bytes(), A.Bytes())
}

func startStubDaemon(t *testing.T, asJSON string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.Contains(req.URL.Path, "get_transactions") {
			http.NotFound(w, req)
			return
		}
		resp := map[string]interface{}{
			"status": "OK",
			"txs": []map[string]interface{}{
				{"tx_hash": testTxID, "as_json": asJSON, "block_height": 1, "in_pool": false},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	rpcClient, err := rpc.NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client = daemon.NewClient(rpcClient)
}

const testTxID = "3c636ac1d28865955f319fb70587d9c30191ca1ff769d6dbde6f546a4a6189e0"

func postJSON(t *testing.T, handler http.HandlerFunc, path string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler(rec, req)
	var out map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestDecodeAndProveEndToEnd(t *testing.T) {
	r := randScalar(t)
	a := randScalar(t)
	b := randScalar(t)
	const amount = uint64(4200000000000)

	asJSON, ourAddr := buildTestTx(t, r, a, b, amount)
	startStubDaemon(t, asJSON)

	rec, out := postJSON(t, HandleDecodeOutputs, "/api/decode-outputs", map[string]string{
		"txid":    testTxID,
		"address": ourAddr,
		"viewkey": hex.EncodeToString(a.Bytes()),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("decode status %d: %v", rec.Code, out)
	}
	if int(out["matched_count"].(float64)) != 1 {
		t.Fatalf("decode matched_count = %v, want 1", out["matched_count"])
	}
	outputs := out["outputs"].([]interface{})
	mine := outputs[1].(map[string]interface{})
	if mine["mine"] != true || mine["amount"] != strconv.FormatUint(amount, 10) || mine["amount_verified"] != true {
		t.Fatalf("decode output 1 = %v", mine)
	}
	if outputs[0].(map[string]interface{})["mine"] != false {
		t.Fatal("unrelated output marked as mine")
	}

	rec, out = postJSON(t, HandleDecodeOutputs, "/api/decode-outputs", map[string]string{
		"txid":    testTxID,
		"address": ourAddr,
		"viewkey": hex.EncodeToString(randScalar(t).Bytes()),
	})
	if rec.Code != http.StatusOK || int(out["matched_count"].(float64)) != 0 {
		t.Fatalf("wrong viewkey should match nothing, got %d %v", rec.Code, out)
	}

	rec, out = postJSON(t, HandleProveSending, "/api/prove-sending", map[string]string{
		"txid":    testTxID,
		"address": ourAddr,
		"txkey":   hex.EncodeToString(r.Bytes()),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("prove status %d: %v", rec.Code, out)
	}
	if out["key_matches_tx"] != true || int(out["matched_count"].(float64)) != 1 || out["total"] != strconv.FormatUint(amount, 10) {
		t.Fatalf("prove result = %v", out)
	}

	rec, _ = postJSON(t, HandleProveSending, "/api/prove-sending", map[string]string{
		"txid":    testTxID,
		"address": ourAddr,
		"txkey":   hex.EncodeToString(randScalar(t).Bytes()),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("prove with wrong key status %d", rec.Code)
	}
}

func TestToolValidation(t *testing.T) {
	rec, _ := postJSON(t, HandleDecodeOutputs, "/api/decode-outputs", map[string]string{"txid": "nothex", "address": "x", "viewkey": "y"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad txid status = %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/prove-sending", nil)
	rec2 := httptest.NewRecorder()
	HandleProveSending(rec2, req)
	if rec2.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d", rec2.Code)
	}

	rec, _ = postJSON(t, HandleProveSending, "/api/prove-sending", map[string]string{
		"txid": testTxID, "address": "892HHTyDg5mJm5eWJWZ8L1ZMYnnWExciQFFkpsgLh1DfVUXfUFj6z1X2jDD2ZRQLiwWYskeyNkrtpAHse4M3G29uBfiYgVL", "txkey": "abcd",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad txkey status = %d", rec.Code)
	}
}
