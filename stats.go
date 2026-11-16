package main

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/cirocosta/go-monero/pkg/rpc/daemon"
)

type statsResponse struct {
	Height          uint64  `json:"height"`
	Difficulty      uint64  `json:"difficulty"`
	Hashrate        uint64  `json:"hashrate"`
	Target          uint64  `json:"target"`
	TxCount         uint64  `json:"tx_count"`
	FeeRate         uint64  `json:"fee_rate"`
	LastBlockTime   int64   `json:"last_block_time"`
	LastBlockReward uint64  `json:"last_block_reward"`
	SampleBlocks    int     `json:"sample_blocks"`
	AvgBlockTime    float64 `json:"avg_block_time"`
	AvgTxsPerBlock  float64 `json:"avg_txs_per_block"`
	AvgBlockSize    float64 `json:"avg_block_size"`
	MedianBlockSize uint64  `json:"median_block_size"`
	BlockSizeLimit  uint64  `json:"block_size_limit"`
	MempoolCount    uint64  `json:"mempool_count"`
	MempoolBytes    uint64  `json:"mempool_bytes"`
	MempoolFees     uint64  `json:"mempool_fees"`
	Outgoing        uint64  `json:"outgoing_peers"`
	Incoming        uint64  `json:"incoming_peers"`
	Synchronized    bool    `json:"synchronized"`
	Version         string  `json:"version"`
}

// sampleBlocks averages block time, transaction count and size over the cached recent blocks.
// Block time is measured across the whole sample, so gaps in the cache don't skew it.
func sampleBlocks(blocks []daemon.BlockHeader) (avgTime, avgTxs, avgSize float64) {
	if len(blocks) < 2 {
		return 0, 0, 0
	}
	sorted := make([]daemon.BlockHeader, len(blocks))
	copy(sorted, blocks)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Height < sorted[j].Height })

	first, last := sorted[0], sorted[len(sorted)-1]
	if heights := last.Height - first.Height; heights > 0 {
		avgTime = float64(last.Timestamp-first.Timestamp) / float64(heights)
	}

	var txs, size float64
	for _, b := range blocks {
		txs += float64(b.NumTxes)
		size += float64(b.BlockSize)
	}
	n := float64(len(blocks))
	return avgTime, txs / n, size / n
}

// fetchStats gathers the network and node figures shown on the home page. The daemon
// client panics on RPC errors, so those are converted to errors here.
func fetchStats() (resp statsResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			resp, err = statsResponse{}, fmt.Errorf("daemon request failed: %v", r)
		}
	}()

	info, err := client.GetInfo(bgCtx)
	if err != nil {
		return resp, err
	}
	last, err := client.GetLastBlockHeader(bgCtx)
	if err != nil {
		return resp, err
	}
	fee, err := client.GetFeeEstimate(bgCtx, 10)
	if err != nil {
		return resp, err
	}
	pool, err := client.GetTransactionPoolStats(bgCtx)
	if err != nil {
		return resp, err
	}

	target := info.Target
	if target == 0 {
		target = uint64(BlockTime)
	}

	avgTime, avgTxs, avgSize := sampleBlocks(RecentBlocks)
	resp = statsResponse{
		Height:          info.Height,
		Difficulty:      info.Difficulty,
		Hashrate:        info.Difficulty / target,
		Target:          target,
		TxCount:         info.TxCount,
		FeeRate:         uint64(fee.Fee),
		LastBlockTime:   last.BlockHeader.Timestamp,
		LastBlockReward: last.BlockHeader.Reward,
		SampleBlocks:    len(RecentBlocks),
		AvgBlockTime:    avgTime,
		AvgTxsPerBlock:  avgTxs,
		AvgBlockSize:    avgSize,
		MedianBlockSize: info.BlockSizeMedian,
		BlockSizeLimit:  info.BlockSizeLimit,
		MempoolCount:    pool.PoolStats.TxsTotal,
		MempoolBytes:    pool.PoolStats.BytesTotal,
		MempoolFees:     pool.PoolStats.FeeTotal,
		Outgoing:        uint64(info.OutgoingConnectionsCount),
		Incoming:        uint64(info.IncomingConnectionsCount),
		Synchronized:    info.Synchronized,
		Version:         info.Version,
	}
	return resp, nil
}

func HandleStats(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeToolJSON(res, http.StatusMethodNotAllowed, toolError{"use GET"})
		return
	}
	resp, err := fetchStats()
	if err != nil {
		writeToolJSON(res, http.StatusInternalServerError, toolError{err.Error()})
		return
	}
	writeToolJSON(res, http.StatusOK, resp)
}
