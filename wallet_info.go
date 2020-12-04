package main

import (
	"encoding/hex"
	"sort"

	"fyne.io/fyne"
	"fyne.io/fyne/dialog"
	"github.com/hectorchu/gonano/rpc"
	"github.com/hectorchu/gonano/util"
	"github.com/hectorchu/gonano/wallet"
	"github.com/tyler-smith/go-bip39"
)

type walletInfo struct {
	w                 *wallet.Wallet
	Label             string
	Seed, Salt        string
	IsBip39, IsLedger bool
	Accounts          map[string]*accountInfo
	accountsList      []*accountInfo
}

type accountInfo struct {
	address          string
	Index            uint32
	balance, pending util.NanoAmount
}

func (wi *walletInfo) init(password string) (err error) {
	if wi.w != nil {
		return
	}
	if wi.IsLedger {
		if err = wi.initLedger(); err != nil {
			return
		}
	} else {
		var seed []byte
		if seed, err = wi.decryptSeed(password); err != nil {
			return
		}
		if wi.IsBip39 {
			if err = wi.initBip39(seed, password); err != nil {
				return
			}
		} else {
			if err = wi.initSeed(seed); err != nil {
				return
			}
		}
	}
	return wi.initAccountsList()
}

func (wi *walletInfo) decryptSeed(password string) (seed []byte, err error) {
	enc, err := hex.DecodeString(wi.Seed)
	if err != nil {
		return
	}
	salt, err := hex.DecodeString(wi.Salt)
	if err != nil {
		return
	}
	key, _, err := deriveKey(password, salt)
	if err != nil {
		return
	}
	return decrypt(enc, key)
}

func (wi *walletInfo) initSeed(seed []byte) (err error) {
	wi.w, err = wallet.NewWallet(seed)
	wi.w.RPC.URL = rpcURL
	return
}

func (wi *walletInfo) initBip39(entropy []byte, password string) (err error) {
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return
	}
	wi.w, err = wallet.NewBip39Wallet(mnemonic, password)
	wi.w.RPC.URL = rpcURL
	return
}

func (wi *walletInfo) initLedger() (err error) {
	wi.w, err = wallet.NewLedgerWallet()
	wi.w.RPC.URL = rpcURL
	return
}

func (wi *walletInfo) initAccounts(win fyne.Window) (err error) {
	prog := dialog.NewProgressInfinite(wi.Label, "Scanning for accounts...", win)
	prog.Show()
	err = wi.w.ScanForAccounts()
	prog.Hide()
	if err != nil {
		return
	}
	if wi.Accounts == nil {
		wi.Accounts = make(map[string]*accountInfo)
	}
	for _, a := range wi.w.GetAccounts() {
		wi.Accounts[a.Address()] = &accountInfo{
			address: a.Address(),
			Index:   a.Index(),
		}
	}
	return wi.initAccountsList()
}

func (wi *walletInfo) initAccountsList() (err error) {
	wi.accountsList = make([]*accountInfo, 0, len(wi.Accounts))
	for _, ai := range wi.Accounts {
		wi.accountsList = append(wi.accountsList, ai)
	}
	sort.Slice(wi.accountsList, func(i, j int) bool {
		return wi.accountsList[i].Index < wi.accountsList[j].Index
	})
	return wi.getBalances()
}

func (wi *walletInfo) indexOf(ai *accountInfo) int {
	return sort.Search(len(wi.accountsList), func(i int) bool {
		return wi.accountsList[i].Index >= ai.Index
	})
}

func (wi *walletInfo) addAccount() (err error) {
	var a *wallet.Account
	for {
		if a, err = wi.w.NewAccount(nil); err != nil {
			return
		}
		if _, ok := wi.Accounts[a.Address()]; !ok {
			break
		}
	}
	ai := &accountInfo{
		address: a.Address(),
		Index:   a.Index(),
	}
	wi.Accounts[a.Address()] = ai
	i := wi.indexOf(ai)
	if i == len(wi.accountsList) {
		wi.accountsList = append(wi.accountsList, ai)
	} else {
		wi.accountsList = append(wi.accountsList[:i+1], wi.accountsList[i:]...)
		wi.accountsList[i] = ai
	}
	rpcClient := rpc.Client{URL: rpcURL}
	balance, pending, err := rpcClient.AccountBalance(ai.address)
	if err != nil {
		return nil
	}
	ai.balance.Raw, ai.pending.Raw = &balance.Int, &pending.Int
	return
}

func (wi *walletInfo) removeAccount(ai *accountInfo) {
	delete(wi.Accounts, ai.address)
	i := wi.indexOf(ai)
	wi.accountsList = append(wi.accountsList[:i], wi.accountsList[i+1:]...)
}

func (wi *walletInfo) getBalances() (err error) {
	if len(wi.accountsList) == 0 {
		return
	}
	accounts := make([]string, len(wi.accountsList))
	for i, ai := range wi.accountsList {
		accounts[i] = ai.address
	}
	rpcClient := rpc.Client{URL: rpcURL}
	balances, err := rpcClient.AccountsBalances(accounts)
	if err != nil {
		return
	}
	for address, ab := range balances {
		ai := wi.Accounts[address]
		ai.balance.Raw, ai.pending.Raw = &ab.Balance.Int, &ab.Pending.Int
	}
	return
}
