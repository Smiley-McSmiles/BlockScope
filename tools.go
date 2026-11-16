package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var txIDRegex = regexp.MustCompile(`^[0-9a-f]{64}$`)

var errTxNotFound = errors.New("transaction not found")

type toolError struct {
	Error string `json:"error"`
}

type decodeOutputsRequest struct {
	TxID    string `json:"txid"`
	Address string `json:"address"`
	ViewKey string `json:"viewkey"`
}

type outputInfo struct {
	Index          int     `json:"index"`
	StealthAddress string  `json:"stealth_address"`
	Mine           bool    `json:"mine"`
	Amount         *string `json:"amount"`
	AmountVerified bool    `json:"amount_verified"`
}

type decodeOutputsResponse struct {
	TxID         string       `json:"txid"`
	Subaddress   bool         `json:"subaddress"`
	MatchedCount int          `json:"matched_count"`
	Total        string       `json:"total"`
	Outputs      []outputInfo `json:"outputs"`
}

type proveSendingRequest struct {
	TxID    string `json:"txid"`
	Address string `json:"address"`
	TxKey   string `json:"txkey"`
}

type proveSendingResponse struct {
	TxID         string       `json:"txid"`
	Subaddress   bool         `json:"subaddress"`
	KeyMatchesTx bool         `json:"key_matches_tx"`
	MatchedCount int          `json:"matched_count"`
	Total        string       `json:"total"`
	Outputs      []outputInfo `json:"outputs"`
}

func writeToolJSON(res http.ResponseWriter, status int, v interface{}) {
	res.Header().Set("Content-Type", "application/json")
	res.Header().Set("Cache-Control", "no-store")
	res.WriteHeader(status)
	json.NewEncoder(res).Encode(v)
}

func readToolRequest(res http.ResponseWriter, req *http.Request, dst interface{}) bool {
	if req.Method != http.MethodPost {
		writeToolJSON(res, http.StatusMethodNotAllowed, toolError{"use POST"})
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(res, req.Body, 1<<14)).Decode(dst); err != nil {
		writeToolJSON(res, http.StatusBadRequest, toolError{"invalid request body"})
		return false
	}
	return true
}

// fetchTxJSON loads a transaction from the daemon and parses its JSON form.
// The daemon client panics on RPC errors, so those are converted to errors here.
func fetchTxJSON(txid string) (tx *txJSON, err error) {
	defer func() {
		if r := recover(); r != nil {
			tx, err = nil, fmt.Errorf("daemon request failed: %v", r)
		}
	}()
	exists, txData := GetTransaction(txid)
	if !exists {
		return nil, errTxNotFound
	}
	var parsed txJSON
	if err := json.Unmarshal([]byte(txData.AsJSON), &parsed); err != nil {
		return nil, fmt.Errorf("could not parse transaction: %w", err)
	}
	return &parsed, nil
}

// outputAmount returns the amount of output index for a derivation. The bool results are
// "verified" (the RingCT commitment matched or the amount is public) and "known".
func outputAmount(tx *txJSON, index int, derivation []byte) (uint64, bool, bool) {
	if tx.RctSignatures.Type == 0 {
		return tx.Vout[index].Amount, true, true
	}
	if index >= len(tx.RctSignatures.EcdhInfo) {
		return 0, false, false
	}
	var commitment []byte
	if index < len(tx.RctSignatures.OutPk) {
		commitment, _ = hex.DecodeString(tx.RctSignatures.OutPk[index])
	}
	amount, verified, err := decryptAmount(sharedSecret(derivation, index), tx.RctSignatures.EcdhInfo[index], commitment)
	if err != nil {
		return 0, false, false
	}
	return amount, verified, true
}

func amountString(amount uint64) *string {
	s := strconv.FormatUint(amount, 10)
	return &s
}

func outputKey(vout txOutput) []byte {
	k, _ := vout.keyBytes()
	return k
}

func HandleDecodeOutputs(res http.ResponseWriter, req *http.Request) {
	var in decodeOutputsRequest
	if !readToolRequest(res, req, &in) {
		return
	}
	txid := strings.ToLower(strings.TrimSpace(in.TxID))
	if !txIDRegex.MatchString(txid) {
		writeToolJSON(res, http.StatusBadRequest, toolError{"invalid transaction id"})
		return
	}
	addr, err := parseMoneroAddress(in.Address)
	if err != nil {
		writeToolJSON(res, http.StatusBadRequest, toolError{err.Error()})
		return
	}
	view, err := parseScalar(strings.TrimSpace(in.ViewKey))
	if err != nil {
		writeToolJSON(res, http.StatusBadRequest, toolError{"invalid private viewkey: " + err.Error()})
		return
	}
	tx, err := fetchTxJSON(txid)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errTxNotFound) {
			status = http.StatusNotFound
		}
		writeToolJSON(res, status, toolError{err.Error()})
		return
	}

	mainKey, additional := parseTxPubKeys(tx.Extra)
	resp := decodeOutputsResponse{TxID: txid, Subaddress: addr.Subaddress, Outputs: []outputInfo{}}
	var total uint64

	for i, vout := range tx.Vout {
		outKey := outputKey(vout)
		info := outputInfo{Index: i, StealthAddress: hex.EncodeToString(outKey)}

		var candidates [][]byte
		if mainKey != nil {
			candidates = append(candidates, mainKey)
		}
		if len(additional) == len(tx.Vout) {
			candidates = append(candidates, additional[i])
		}
		for _, R := range candidates {
			point, err := parsePointBytes(R)
			if err != nil {
				continue
			}
			derivation := deriveSecret(view, point)
			if !outputMatches(derivation, i, outKey, addr.Spend) {
				continue
			}
			info.Mine = true
			if amount, verified, known := outputAmount(tx, i, derivation); known {
				info.Amount = amountString(amount)
				info.AmountVerified = verified
				total += amount
			}
			break
		}

		if info.Mine {
			resp.MatchedCount++
		}
		resp.Outputs = append(resp.Outputs, info)
	}
	resp.Total = strconv.FormatUint(total, 10)

	writeToolJSON(res, http.StatusOK, resp)
}

func HandleProveSending(res http.ResponseWriter, req *http.Request) {
	var in proveSendingRequest
	if !readToolRequest(res, req, &in) {
		return
	}
	txid := strings.ToLower(strings.TrimSpace(in.TxID))
	if !txIDRegex.MatchString(txid) {
		writeToolJSON(res, http.StatusBadRequest, toolError{"invalid transaction id"})
		return
	}
	addr, err := parseMoneroAddress(in.Address)
	if err != nil {
		writeToolJSON(res, http.StatusBadRequest, toolError{err.Error()})
		return
	}
	txKey, err := parseScalar(strings.TrimSpace(in.TxKey))
	if err != nil {
		writeToolJSON(res, http.StatusBadRequest, toolError{"invalid tx private key: " + err.Error()})
		return
	}
	tx, err := fetchTxJSON(txid)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errTxNotFound) {
			status = http.StatusNotFound
		}
		writeToolJSON(res, status, toolError{err.Error()})
		return
	}

	derivation := deriveSecret(txKey, addr.View)
	resp := proveSendingResponse{TxID: txid, Subaddress: addr.Subaddress, Outputs: []outputInfo{}}
	var total uint64

	for i, vout := range tx.Vout {
		outKey := outputKey(vout)
		if !outputMatches(derivation, i, outKey, addr.Spend) {
			continue
		}
		info := outputInfo{Index: i, StealthAddress: hex.EncodeToString(outKey), Mine: true}
		if amount, verified, known := outputAmount(tx, i, derivation); known {
			info.Amount = amountString(amount)
			info.AmountVerified = verified
			total += amount
		}
		resp.Outputs = append(resp.Outputs, info)
	}
	resp.MatchedCount = len(resp.Outputs)
	resp.Total = strconv.FormatUint(total, 10)

	// The tx key should equal the transaction's public key (r*G) or, for subaddress
	// recipients, one of the additional keys (r*B).
	rG := edwardsBaseMult(txKey)
	rB := edwardsMult(txKey, addr.Spend)
	mainKey, additional := parseTxPubKeys(tx.Extra)
	candidates := append([][]byte{mainKey}, additional...)
	for _, k := range candidates {
		if k != nil && (bytes.Equal(k, rG) || bytes.Equal(k, rB)) {
			resp.KeyMatchesTx = true
			break
		}
	}

	writeToolJSON(res, http.StatusOK, resp)
}
