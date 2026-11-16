package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/cirocosta/go-monero/pkg/rpc/daemon"
)

type mempoolTx struct {
	ID          string `json:"id"`
	Fee         uint64 `json:"fee"`
	Size        uint64 `json:"size"`
	FeeRate     uint64 `json:"fee_rate"`
	Received    int64  `json:"received"`
	DoubleSpend bool   `json:"double_spend_seen"`
}

type mempoolResponse struct {
	Count        int         `json:"count"`
	TotalSize    uint64      `json:"total_size"`
	TotalFees    string      `json:"total_fees"`
	Oldest       int64       `json:"oldest"`
	Limited      bool        `json:"limited"`
	Transactions []mempoolTx `json:"transactions"`
}

// restrictedBatchSize keeps each get_transactions request small enough for a restricted node.
const restrictedBatchSize = 100

// fetchMempool loads the full transaction pool. Restricted RPC ports don't serve
// this endpoint, so callers should fall back to fetchRestrictedMempool on error.
// The daemon client panics on RPC errors, so those are converted to errors here.
func fetchMempool() (pool *daemon.GetTransactionPoolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			pool, err = nil, fmt.Errorf("daemon request failed: %v", r)
		}
	}()
	return client.GetTransactionPool(bgCtx)
}

func buildMempool(pool *daemon.GetTransactionPoolResult) mempoolResponse {
	resp := mempoolResponse{Transactions: []mempoolTx{}}
	var fees uint64
	for _, t := range pool.Transactions {
		if !txIDRegex.MatchString(t.IDHash) {
			continue
		}
		weight := t.Weight
		if weight == 0 {
			weight = t.BlobSize
		}
		var rate uint64
		if weight > 0 {
			rate = t.Fee / weight
		}
		resp.Transactions = append(resp.Transactions, mempoolTx{
			ID:          t.IDHash,
			Fee:         t.Fee,
			Size:        t.BlobSize,
			FeeRate:     rate,
			Received:    t.ReceiveTime,
			DoubleSpend: t.DoubleSpendSeen,
		})
		resp.TotalSize += t.BlobSize
		fees += t.Fee
		if resp.Oldest == 0 || t.ReceiveTime < resp.Oldest {
			resp.Oldest = t.ReceiveTime
		}
	}
	resp.Count = len(resp.Transactions)
	resp.TotalFees = strconv.FormatUint(fees, 10)
	sortByFeeRate(resp.Transactions)
	return resp
}

func sortByFeeRate(txs []mempoolTx) {
	sort.SliceStable(txs, func(i, j int) bool {
		return txs[i].FeeRate > txs[j].FeeRate
	})
}

// fetchRestrictedMempool builds the mempool from the calls a restricted RPC port serves:
// the pool statistics, the list of pool hashes, and get_transactions for each hash.
// Receive times aren't available on this path, so the response is marked limited.
func fetchRestrictedMempool() (resp mempoolResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			resp, err = mempoolResponse{}, fmt.Errorf("daemon request failed: %v", r)
		}
	}()

	stats, err := client.GetTransactionPoolStats(bgCtx)
	if err != nil {
		return resp, err
	}
	var hashes struct {
		Hashes []string `json:"tx_hashes"`
	}
	if err := client.RawRequest(bgCtx, "/get_transaction_pool_hashes", map[string]interface{}{}, &hashes); err != nil {
		return resp, err
	}

	resp = mempoolResponse{Transactions: []mempoolTx{}, Limited: true, Oldest: stats.PoolStats.Oldest}
	var fees uint64
	for start := 0; start < len(hashes.Hashes); start += restrictedBatchSize {
		end := start + restrictedBatchSize
		if end > len(hashes.Hashes) {
			end = len(hashes.Hashes)
		}
		txs, err := client.GetTransactions(bgCtx, hashes.Hashes[start:end])
		if err != nil {
			return resp, err
		}
		for _, t := range txs.Txs {
			if !txIDRegex.MatchString(t.TxHash) {
				continue
			}
			var parsed txJSON
			if err := json.Unmarshal([]byte(t.AsJSON), &parsed); err != nil {
				continue
			}
			size := uint64(len(t.AsHex) / 2)
			fee := parsed.RctSignatures.TxnFee
			var rate uint64
			if size > 0 {
				rate = fee / size
			}
			resp.Transactions = append(resp.Transactions, mempoolTx{
				ID:          t.TxHash,
				Fee:         fee,
				Size:        size,
				FeeRate:     rate,
				DoubleSpend: t.DoubleSpendSeen,
			})
			resp.TotalSize += size
			fees += fee
		}
	}
	resp.Count = len(resp.Transactions)
	resp.TotalFees = strconv.FormatUint(fees, 10)
	sortByFeeRate(resp.Transactions)
	return resp, nil
}

func HandleMempool(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeToolJSON(res, http.StatusMethodNotAllowed, toolError{"use GET"})
		return
	}
	pool, err := fetchMempool()
	if err == nil {
		writeToolJSON(res, http.StatusOK, buildMempool(pool))
		return
	}
	resp, restrictedErr := fetchRestrictedMempool()
	if restrictedErr != nil {
		writeToolJSON(res, http.StatusInternalServerError, toolError{
			fmt.Sprintf("could not load the mempool: %v (restricted fallback: %v)", err, restrictedErr),
		})
		return
	}
	writeToolJSON(res, http.StatusOK, resp)
}
