// Copyright (c) 2022, duggavo
// Copyright (c) 2026, WOOSAH
//
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without modification, are
// permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this list of
//    conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice, this list
//    of conditions and the following disclaimer in the documentation and/or other
//    materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its contributors may be
//    used to endorse or promote products derived from this software without specific
//    prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY
// EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL
// THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
// STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF
// THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package main

import (
	"flag"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cirocosta/go-monero/pkg/rpc/daemon"
)

const Version string = "0.1.4"

// Monero network settings
const BlockTime int = 120

var listenAddress, ProxyToUse, DaemonUrl, RpcLogin string

var RpcPassword, RpcUsername string

func main() {
	fmt.Println("\x1b[1m\x1b[32m" + `
  ____  _            _      ____
 | __ )| | ___   ___| | __ / ___|  ___ ___  _ __   ___
 |  _ \| |/ _ \ / __| |/ / \___ \ / __/ _ \| '_ \ / _ \
 | |_) | | (_) | (__|   <   ___) | (_| (_) | |_) |  __/
 |____/|_|\___/ \___|_|\_\ |____/ \___\___/| .__/ \___|
                                           |_|

                 Your node, Your ledger, No middlemen` + "\x1b[0m")
	fmt.Println("\n\nStarting BlockScope v" + Version)

	flag.StringVar(&listenAddress, "bind", "127.0.0.1:31312", "Address and port to bind.")
	flag.StringVar(&ProxyToUse, "proxy", "none", "Proxy to use. Should start with socks5://, socks4:// or http:// .")
	flag.StringVar(&RpcLogin, "rpc-login", "none", "Required if daemon has login enabled.")
	flag.StringVar(&DaemonUrl, "daemon", "127.0.0.1:18081", "The Monero daemon URL. Please note that using a third-party daemon might harm privacy if you do not use a proxy.")
	flag.Parse()

	if RpcLogin != "none" {
		splLogin := strings.Split(RpcLogin, ":")
		if len(splLogin) < 2 {
			fmt.Println("rpc-login flag is not valid. Ignoring it.")
			RpcLogin = "none"
		} else {
			RpcPassword = splLogin[0]
			RpcUsername = splLogin[1]
		}
	}

	if !strings.HasPrefix(DaemonUrl, "http://") {
		DaemonUrl = "http://" + DaemonUrl
	}

	InitPages()
	StartDaemonRpc()

	http.HandleFunc("/style.css", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "text/css; charset=utf-8")
		res.Header().Set("Cache-Control", "public, max-age=3600")
		res.Write(StyleSheetPage)
	})
	http.HandleFunc("/logo.svg", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "image/svg+xml")
		res.Header().Set("Cache-Control", "public, max-age=3600")
		res.Write(LogoSVG)
	})
	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		var blocksList strings.Builder
		for i := len(RecentBlocks) - 1; i >= 0; i-- {
			e := RecentBlocks[i]
			CheckString(e.Hash)
			blocksList.WriteString(`<tr class="tr"><td>`)
			blocksList.WriteString(strconv.FormatUint(e.Height, 10))
			blocksList.WriteString(`</td><td>`)
			blocksList.WriteString(strconv.FormatUint(uint64(e.NumTxes), 10))
			blocksList.WriteString(`</td><td><a class="monos" href="/block?id=`)
			blocksList.WriteString(e.Hash)
			blocksList.WriteString(`">`)
			blocksList.WriteString(e.Hash)
			blocksList.WriteString(`</a></td><td>`)
			blocksList.WriteString(FormatTimeAgo(time.Now().Unix() - e.Timestamp))
			blocksList.WriteString(`</td></tr>`)
		}

		pageOut := strings.Replace(MainPage, "$blocks", blocksList.String(), 1)

		res.Header().Set("Content-Type", "text/html; charset=utf-8")
		res.Write([]byte(pageOut))
	})
	http.HandleFunc("/search", func(res http.ResponseWriter, req *http.Request) {
		sParams := req.URL.Query()["q"]
		if len(sParams) == 0 || strings.TrimSpace(sParams[0]) == "" {
			renderError(res, http.StatusBadRequest, "Missing search query", "Enter a block height, block hash, or transaction ID to search.")
			return
		}
		searchParam := strings.TrimSpace(sParams[0])
		searchType := GetSearchType(searchParam)

		if !CheckString2(searchParam) || searchType == 2 {
			renderError(res, http.StatusNotFound, "Not found", "No matching transaction or block was found.")
			return
		}

		if searchType == 0 {
			http.Redirect(res, req, "/tx?id="+url.QueryEscape(searchParam), http.StatusFound)
		} else if searchType == 1 {
			http.Redirect(res, req, "/block?id="+url.QueryEscape(searchParam), http.StatusFound)
		}
	})
	http.HandleFunc("/tx", func(res http.ResponseWriter, req *http.Request) {
		sParams := req.URL.Query()["id"]
		if len(sParams) == 0 || sParams[0] == "" {
			renderError(res, http.StatusBadRequest, "Missing transaction ID", "Provide a transaction ID via ?id=...")
			return
		}
		txid := sParams[0]
		if !CheckString2(txid) {
			renderError(res, http.StatusNotFound, "Transaction not found", "The requested transaction could not be found.")
			return
		}

		exists, txData := GetTransaction(txid)
		if !exists {
			renderError(res, http.StatusNotFound, "Transaction not found", "The requested transaction could not be found.")
			return
		}

		confirmations := strconv.FormatUint(GetHeight()-txData.BlockHeight, 10)
		if txData.BlockHeight == 0 {
			confirmations = "0"
		}
		CheckString(txData.TxHash)

		o := strings.Replace(TxPage, "$confirmations", confirmations, 1)
		height := strconv.FormatUint(txData.BlockHeight, 10)
		o = strings.Replace(o, "$height", height, 2)
		txSize := strconv.FormatFloat(float64(len(txData.AsHex))/2/1000, 'f', 2, 64)
		o = strings.Replace(o, "$size", txSize, 1)
		o = strings.Replace(o, "$txid", txData.TxHash, 1)

		o = strings.Replace(o, "$doublespend", strconv.FormatBool(txData.DoubleSpendSeen), 1)
		formattedTime := time.Unix(txData.BlockTimestamp, 0).Format("2006-01-02 15:04")
		o = strings.Replace(o, "$timestamp", formattedTime, 1)

		res.Header().Set("Content-Type", "text/html; charset=utf-8")
		res.Write([]byte(o))
	})
	http.HandleFunc("/block", func(res http.ResponseWriter, req *http.Request) {
		sParams := req.URL.Query()["id"]
		if len(sParams) == 0 || sParams[0] == "" {
			renderError(res, http.StatusBadRequest, "Missing block ID", "Provide a block height or hash via ?id=...")
			return
		}
		blockId := sParams[0]
		isNumber, _ := regexp.Match(`^\d+$`, []byte(blockId))

		var params daemon.GetBlockRequestParameters
		if isNumber {
			blockNum, _ := strconv.ParseUint(blockId, 10, 64)
			params = daemon.GetBlockRequestParameters{Height: blockNum}

		} else {
			params = daemon.GetBlockRequestParameters{Hash: blockId}
		}
		if !CheckString2(blockId) {
			renderError(res, http.StatusNotFound, "Block not found", "The requested block could not be found.")
			return
		}

		exists, blockD, blockBody := GetBlock(params)
		if !exists || blockD == nil {
			renderError(res, http.StatusNotFound, "Block not found", "The requested block could not be found.")
			return
		}
		blockData := blockD.BlockHeader
		CheckString(blockData.Hash)
		CheckString(blockD.MinerTxHash)
		transactions := append(blockBody.TxHashes, blockD.MinerTxHash)
		o := strings.Replace(BlockPage, "$blocknum", strconv.FormatUint(blockData.Height, 10), 1)
		o = strings.Replace(o, "$hash", blockData.Hash, 1)
		o = strings.Replace(o, "$numtxes", strconv.FormatUint(uint64(blockData.NumTxes), 10), 1)
		o = strings.Replace(o, "$size", strconv.FormatFloat(float64(blockData.BlockSize)/1000, 'f', 1, 64), 1)
		o = strings.Replace(o, "$timestamp", time.Unix(blockData.Timestamp, 0).Format("2006-01-02 15:04"), 1)
		o = strings.Replace(o, "$diff", strconv.FormatUint(blockData.Difficulty/1000/1000, 10), 1)
		o = strings.Replace(o, "$reward", strconv.FormatFloat(float64(blockData.Reward)/math.Pow(10, 12), 'f', 5, 64), 1)
		o = strings.Replace(o, "$minerTx", blockD.MinerTxHash, 2)

		var txList strings.Builder
		for i, e := range transactions {
			CheckString(e)
			txList.WriteString(`<tr class="tr"><td>`)
			txList.WriteString(strconv.FormatInt(int64(i+1), 10))
			txList.WriteString(`</td><td><a href="/tx?id=`)
			txList.WriteString(e)
			txList.WriteString(`" class="monos">`)
			txList.WriteString(e)
			txList.WriteString(`</a>`)
			if e == blockD.MinerTxHash {
				txList.WriteString(`<span class="badge subtle">Coinbase</span>`)
			}
			txList.WriteString(`</td></tr>`)
		}
		o = strings.Replace(o, "$txList", txList.String(), 1)

		res.Header().Set("Content-Type", "text/html; charset=utf-8")
		res.Write([]byte(o))
	})

	http.HandleFunc("/api/decode-outputs", HandleDecodeOutputs)
	http.HandleFunc("/api/prove-sending", HandleProveSending)
	http.HandleFunc("/api/mempool", HandleMempool)
	http.HandleFunc("/api/stats", HandleStats)
	http.HandleFunc("/mempool", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "text/html; charset=utf-8")
		res.Write([]byte(MempoolPage))
	})

	srv := &http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  80 * time.Second,
		Addr:         listenAddress,
	}
	paddedAddress := listenAddress + "/"
	for {
		if len(paddedAddress) < 16 {
			paddedAddress += " "
		} else {
			break
		}
	}
	fmt.Println("Server listening at http://" + listenAddress + "/")
	srv.ListenAndServe()

}

func renderError(res http.ResponseWriter, status int, title, message string) {
	res.Header().Set("Content-Type", "text/html; charset=utf-8")
	res.WriteHeader(status)
	body := Header + `<section class="card fade"><div class="card-head"><h2>` + html.EscapeString(title) + `</h2></div><p class="muted">` + html.EscapeString(message) + `</p><p><a class="btn-ghost" href="/">← Back to home</a></p></section>` + Footer
	res.Write([]byte(body))
}

func FormatTimeAgo(t int64) string {
	if t > 60*60*24*365*2 {
		return strconv.FormatInt(t/60/60/24/365, 10) + " y"

	}
	if t > 60*60*24 {
		return strconv.FormatInt(t/60/60/24, 10) + " d"
	} else if t > 60*60 {
		return strconv.FormatInt(t/60/60, 10) + " h"
	} else if t > 60 {
		return strconv.FormatInt(t/60, 10) + " min"
	} else {
		return strconv.FormatInt(t, 10) + " sec"
	}
}

func CheckString(s string) {
	if strings.Contains(s, "<") || strings.Contains(s, ">") || strings.Contains(s, "&") {
		panic("Daemon sent unsafe data (containing characters '<' '>' '&'). It should not be trusted.")
	}
}
func CheckString2(s string) bool {
	if strings.Contains(s, "<") || strings.Contains(s, ">") || strings.Contains(s, "&") {
		return false
	}
	return true
}
