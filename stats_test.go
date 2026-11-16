package main

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cirocosta/go-monero/pkg/rpc"
	"github.com/cirocosta/go-monero/pkg/rpc/daemon"
)

func TestStatsFromDaemon(t *testing.T) {
	const difficulty = uint64(846503637693)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var rpcReq struct {
			Method string `json:"method"`
		}
		json.NewDecoder(req.Body).Decode(&rpcReq)
		path := req.URL.Path
		if strings.HasSuffix(path, "/json_rpc") {
			path = "/" + rpcReq.Method
		}
		var body interface{}
		switch {
		case strings.HasSuffix(path, "/get_info"):
			body = map[string]interface{}{
				"status": "OK", "height": 3807905, "difficulty": difficulty, "tx_count": 65633000,
				"outgoing_connections_count": 8, "incoming_connections_count": 12,
				"synchronized": true, "block_size_median": 60000, "block_size_limit": 120000, "version": "0.18.4.0",
			}
		case strings.HasSuffix(path, "/get_last_block_header"):
			body = map[string]interface{}{
				"status":       "OK",
				"block_header": map[string]interface{}{"height": 3807904, "timestamp": 1700000000, "reward": 600000000000},
			}
		case strings.HasSuffix(path, "/get_fee_estimate"):
			body = map[string]interface{}{"status": "OK", "fee": 80000, "quantization_mask": 1000}
		case strings.HasSuffix(path, "/get_transaction_pool_stats"):
			body = map[string]interface{}{
				"status":     "OK",
				"pool_stats": map[string]interface{}{"txs_total": 6, "bytes_total": 10770, "fee_total": 307000},
			}
		default:
			http.NotFound(w, req)
			return
		}
		if strings.HasSuffix(req.URL.Path, "/json_rpc") {
			body = map[string]interface{}{"jsonrpc": "2.0", "id": "0", "result": body}
		}
		json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)

	rpcClient, err := rpc.NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client = daemon.NewClient(rpcClient)

	saved := RecentBlocks
	t.Cleanup(func() { RecentBlocks = saved })
	RecentBlocks = []daemon.BlockHeader{
		{Height: 100, Timestamp: 1000, NumTxes: 10, BlockSize: 10000},
		{Height: 101, Timestamp: 1130, NumTxes: 30, BlockSize: 30000},
		{Height: 102, Timestamp: 1240, NumTxes: 20, BlockSize: 20000},
	}

	res := httptest.NewRecorder()
	HandleStats(res, httptest.NewRequest(http.MethodGet, "/api/stats", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	var s statsResponse
	if err := json.Unmarshal(res.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}

	if s.Height != 3807905 || s.Difficulty != difficulty || s.Hashrate != difficulty/uint64(BlockTime) {
		t.Fatalf("height/difficulty/hashrate = %d %d %d", s.Height, s.Difficulty, s.Hashrate)
	}
	if s.FeeRate != 80000 || s.MempoolCount != 6 || s.MempoolFees != 307000 || s.MempoolBytes != 10770 {
		t.Fatalf("fee/mempool = %d %d %d %d", s.FeeRate, s.MempoolCount, s.MempoolFees, s.MempoolBytes)
	}
	if s.Outgoing != 8 || s.Incoming != 12 || !s.Synchronized || s.Version != "0.18.4.0" {
		t.Fatalf("node = %d %d %v %s", s.Outgoing, s.Incoming, s.Synchronized, s.Version)
	}
	if s.LastBlockTime != 1700000000 || s.LastBlockReward != 600000000000 {
		t.Fatalf("last block = %d %d", s.LastBlockTime, s.LastBlockReward)
	}
	// (1240 - 1000) / (102 - 100) = 120 s per block
	if math.Abs(s.AvgBlockTime-120) > 1e-9 {
		t.Fatalf("avg block time = %v, want 120", s.AvgBlockTime)
	}
	if math.Abs(s.AvgTxsPerBlock-20) > 1e-9 || math.Abs(s.AvgBlockSize-20000) > 1e-9 {
		t.Fatalf("averages = %v txs, %v bytes", s.AvgTxsPerBlock, s.AvgBlockSize)
	}
}

func TestStatsRejectsPost(t *testing.T) {
	res := httptest.NewRecorder()
	HandleStats(res, httptest.NewRequest(http.MethodPost, "/api/stats", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", res.Code)
	}
}
