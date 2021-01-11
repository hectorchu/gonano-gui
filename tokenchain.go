package main

import (
	"bytes"
	"database/sql"
	"encoding/hex"
	"errors"
	"math/big"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hectorchu/gonano/rpc"
	"github.com/hectorchu/gonano/wallet"
	"github.com/hectorchu/nano-token-protocol/tokenchain"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

type tokenChainManager struct {
	m, mDB sync.Mutex
	chains map[string]*tokenchain.Chain
	tokens map[string]*tokenchain.Token
}

func newTokenChainManager() (tcm *tokenChainManager) {
	tcm = &tokenChainManager{
		chains: make(map[string]*tokenchain.Chain),
		tokens: make(map[string]*tokenchain.Token),
	}
	go func() {
		for range time.Tick(10 * time.Second) {
			tcm.parse()
		}
	}()
	return
}

func (tcm *tokenChainManager) getTokens() (tokens []*tokenchain.Token) {
	tokens = make([]*tokenchain.Token, 0, len(tcm.tokens))
	for _, token := range tcm.tokens {
		tokens = append(tokens, token)
	}
	sort.Slice(tokens, func(i, j int) bool {
		return bytes.Compare(tokens[i].Hash(), tokens[j].Hash()) < 0
	})
	return
}

func (tcm *tokenChainManager) getBalance(token *tokenchain.Token, account string) (balance *big.Int) {
	tcm.m.Lock()
	balance = token.Balance(account)
	tcm.m.Unlock()
	return
}

func (tcm *tokenChainManager) createToken(
	chain *tokenchain.Chain, a *wallet.Account, name string, supply *big.Int, decimals byte,
) (token *tokenchain.Token, err error) {
	if chain == nil {
		if chain, err = tokenchain.NewChain(rpcURL); err != nil {
			return
		}
		if _, err = a.Send(chain.Address(), big.NewInt(1)); err != nil {
			return
		}
		if err = chain.WaitForOpen(); err != nil {
			return
		}
		tcm.m.Lock()
		tcm.chains[chain.Address()] = chain
		tcm.m.Unlock()
	}
	client := rpc.Client{URL: rpcURL}
	rep, err := client.AccountRepresentative(a.Address())
	if err != nil {
		return
	}
	tcm.m.Lock()
	token, err = tokenchain.TokenGenesis(chain, a, name, supply, decimals)
	tcm.m.Unlock()
	if err != nil {
		return
	}
	if _, err = a.ChangeRep(rep); err != nil {
		return
	}
	tcm.tokens[string(token.Hash())] = token
	return
}

func (tcm *tokenChainManager) transferToken(
	token *tokenchain.Token, a *wallet.Account, account string, amount *big.Int,
) (hash rpc.BlockHash, err error) {
	client := rpc.Client{URL: rpcURL}
	rep, err := client.AccountRepresentative(a.Address())
	if err != nil {
		return
	}
	tcm.m.Lock()
	hash, err = token.Transfer(a, account, amount)
	tcm.m.Unlock()
	if err != nil {
		return
	}
	_, err = a.ChangeRep(rep)
	return
}

func (tcm *tokenChainManager) fetchChain(address string) (chain *tokenchain.Chain, err error) {
	chain, ok := tcm.chains[address]
	if !ok {
		if chain, err = tokenchain.LoadChain(address, rpcURL); err != nil {
			return
		}
		tcm.m.Lock()
		tcm.chains[address] = chain
	} else {
		tcm.m.Lock()
	}
	err = chain.Parse()
	tcm.m.Unlock()
	if err != nil {
		return
	}
	err = tcm.withDB(func(db *sql.DB) error { return chain.SaveState(db) })
	return
}

func (tcm *tokenChainManager) fetchToken(hash rpc.BlockHash) (token *tokenchain.Token, err error) {
	token, ok := tcm.tokens[string(hash)]
	if ok {
		return
	}
	client := rpc.Client{URL: rpcURL}
	block, err := client.BlockInfo(hash)
	if err != nil {
		return
	}
	chain, err := tcm.fetchChain(block.BlockAccount)
	if err != nil {
		return
	}
	tcm.m.Lock()
	token, err = chain.Token(hash)
	tcm.m.Unlock()
	if err != nil {
		return
	}
	tcm.tokens[string(hash)] = token
	return
}

func (tcm *tokenChainManager) parse() (err error) {
	tcm.m.Lock()
	for _, chain := range tcm.chains {
		if err = chain.Parse(); err != nil {
			break
		}
		if err = tcm.withDB(func(db *sql.DB) error { return chain.SaveState(db) }); err != nil {
			break
		}
	}
	tcm.m.Unlock()
	return
}

func (tcm *tokenChainManager) withDB(cb func(*sql.DB) error) (err error) {
	home, err := homedir.Dir()
	if err != nil {
		return
	}
	tcm.mDB.Lock()
	defer tcm.mDB.Unlock()
	db, err := sql.Open("sqlite3", filepath.Join(home, "tokenchains.db"))
	if err != nil {
		return
	}
	defer db.Close()
	return cb(db)
}

func (tcm *tokenChainManager) load() (err error) {
	tcm.withDB(func(db *sql.DB) error { return tcm.loadChains(db) })
	tokens := viper.GetStringSlice("tokens")
	for _, hash := range tokens {
		var h rpc.BlockHash
		if h, err = hex.DecodeString(hash); err != nil {
			return
		}
		for _, chain := range tcm.chains {
			if token, err := chain.Token(h); err == nil {
				tcm.tokens[string(h)] = token
				break
			}
		}
		if _, err = tcm.fetchToken(h); err != nil {
			return
		}
	}
	return
}

func (tcm *tokenChainManager) loadChains(db *sql.DB) (err error) {
	rows, err := db.Query("SELECT seed FROM chains")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var seedStr string
		if err := rows.Scan(&seedStr); err != nil {
			return err
		}
		seed, err := hex.DecodeString(seedStr)
		if err != nil {
			return err
		}
		chain, err := tokenchain.NewChainFromSeed(seed, rpcURL)
		if err != nil {
			return err
		}
		if err = chain.LoadState(db); err != nil {
			return err
		}
		tcm.chains[chain.Address()] = chain
	}
	return rows.Err()
}

func (tcm *tokenChainManager) save() (err error) {
	tokens := make([]string, 0, len(tcm.tokens))
	for hash := range tcm.tokens {
		tokens = append(tokens, strings.ToUpper(hex.EncodeToString(rpc.BlockHash(hash))))
	}
	viper.Set("tokens", tokens)
	return viper.WriteConfig()
}

func (tcm *tokenChainManager) amountToString(amount *big.Int, decimals byte) string {
	x := big.NewInt(10)
	exp := x.Exp(x, big.NewInt(int64(decimals)), nil)
	r := new(big.Rat).SetFrac(amount, exp)
	return r.FloatString(int(decimals))
}

func (tcm *tokenChainManager) amountFromString(s string, decimals byte) (amount *big.Int, err error) {
	x := big.NewInt(10)
	exp := x.Exp(x, big.NewInt(int64(decimals)), nil)
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, errors.New("Unable to parse amount")
	}
	r = r.Mul(r, new(big.Rat).SetInt(exp))
	if !r.IsInt() {
		return nil, errors.New("Unable to parse amount")
	}
	return r.Num(), nil
}
