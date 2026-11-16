package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cirocosta/go-monero/pkg/rpc"
	"github.com/cirocosta/go-monero/pkg/rpc/daemon"
)

func TestMempoolSummaryAndOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.Contains(req.URL.Path, "get_transaction_pool") {
			http.NotFound(w, req)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "OK",
			"transactions": []map[string]interface{}{
				{"id_hash": strings.Repeat("a", 64), "fee": 600000, "weight": 1500, "blob_size": 1530, "receive_time": 1000},
				{"id_hash": strings.Repeat("b", 64), "fee": 4800000, "weight": 2000, "blob_size": 2220, "receive_time": 900},
				{"id_hash": "not-a-hash", "fee": 1, "weight": 1, "blob_size": 1, "receive_time": 1},
			},
		})
	}))
	t.Cleanup(srv.Close)

	rpcClient, err := rpc.NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client = daemon.NewClient(rpcClient)

	res := httptest.NewRecorder()
	HandleMempool(res, httptest.NewRequest(http.MethodGet, "/api/mempool", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	var out mempoolResponse
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 {
		t.Fatalf("count = %d, want 2 (malformed id dropped)", out.Count)
	}
	if out.Transactions[0].ID != strings.Repeat("b", 64) {
		t.Fatal("transactions not sorted by fee rate")
	}
	if out.Transactions[0].FeeRate != 2400 || out.Transactions[1].FeeRate != 400 {
		t.Fatalf("fee rates = %d, %d", out.Transactions[0].FeeRate, out.Transactions[1].FeeRate)
	}
	if out.TotalSize != 1530+2220 || out.TotalFees != "5400000" || out.Oldest != 900 {
		t.Fatalf("totals = size %d fees %s oldest %d", out.TotalSize, out.TotalFees, out.Oldest)
	}
}

func TestMempoolRejectsPost(t *testing.T) {
	res := httptest.NewRecorder()
	HandleMempool(res, httptest.NewRequest(http.MethodPost, "/api/mempool", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", res.Code)
	}
}
