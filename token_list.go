package main

import (
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne"
	"fyne.io/fyne/container"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	"github.com/hectorchu/gonano/wallet"
	"github.com/hectorchu/nano-token-protocol/tokenchain"
)

var tcm = newTokenChainManager()

type tokenList struct {
	wi             *walletInfo
	ai             *accountInfo
	list           *widget.List
	newTokenButton *widget.Button
	addTokenButton *widget.Button
	transferButton *widget.Button
	selectedToken  *tokenchain.Token
}

func newTokenList(wi *walletInfo, ai *accountInfo) (tl *tokenList) {
	win := fyne.CurrentApp().NewWindow("Tokens for " + ai.address)
	tl = &tokenList{
		wi: wi,
		ai: ai,
		list: widget.NewList(
			func() int { return len(tcm.getTokens()) },
			func() fyne.CanvasObject {
				return fyne.NewContainerWithLayout(
					newHBoxLayout([]int{600, 120}), newCopyableLabel(win, ""),
					newCopyableLabel(win, ""), newCopyableLabel(win, ""),
				)
			},
			func(id widget.ListItemID, item fyne.CanvasObject) {
				tokens := tcm.getTokens()
				if id >= len(tokens) {
					return
				}
				token := tokens[id]
				getLabel := func(i int) *contextMenuLabel {
					return item.(*fyne.Container).Objects[i].(*contextMenuLabel)
				}
				getLabel(0).SetText(strings.ToUpper(hex.EncodeToString(token.Hash())))
				getLabel(1).SetText(token.Name())
				getLabel(2).SetText(tcm.amountToString(tcm.getBalance(token, ai.address), token.Decimals()))
				getLabel(0).tapped = func() { tl.list.Select(id) }
				getLabel(1).tapped = func() { tl.list.Select(id) }
				getLabel(2).tapped = func() { tl.list.Select(id) }
			},
		),
		newTokenButton: widget.NewButtonWithIcon("New Token", theme.DocumentCreateIcon(), func() {
			tl.showNewTokenDialog(win)
		}),
		addTokenButton: widget.NewButtonWithIcon("Add Existing Token", theme.ContentAddIcon(), func() {
			tl.showAddTokenDialog(win)
		}),
		transferButton: widget.NewButtonWithIcon("Transfer", theme.MailForwardIcon(), func() {
			tl.showTransferDialog(win)
		}),
	}
	tl.list.OnSelected = func(id widget.ListItemID) { tl.setToken(tcm.getTokens()[id]) }
	tl.list.OnUnselected = func(id widget.ListItemID) { tl.setToken(nil) }
	tl.setToken(nil)
	win.SetContent(container.NewBorder(
		widget.NewLabel("Tokens:"),
		widget.NewHBox(tl.newTokenButton, tl.addTokenButton, tl.transferButton),
		nil, nil, tl.list,
	))
	win.Resize(fyne.NewSize(1000, 400))
	win.CenterOnScreen()
	win.Show()
	done := make(chan bool)
	win.SetOnClosed(func() { done <- true })
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-ticker.C:
				tl.list.Refresh()
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()
	return
}

func (tl *tokenList) setToken(token *tokenchain.Token) {
	tl.selectedToken = token
	if token != nil {
		tl.transferButton.Enable()
	} else {
		tl.transferButton.Disable()
	}
}

func (tl *tokenList) getAccount() (a *wallet.Account, err error) {
	if a, err = tl.wi.w.NewAccount(&tl.ai.Index); err != nil {
		return
	}
	if a.Address() != tl.ai.address {
		err = errors.New("Address mismatch")
	}
	return
}

func (tl *tokenList) showNewTokenDialog(win fyne.Window) {
	var (
		name     = widget.NewEntry()
		supply   = widget.NewEntry()
		decimals = widget.NewEntry()
		scroll   = container.NewHScroll(name)
		content  = widget.NewForm(
			widget.NewFormItem("Name", scroll),
			widget.NewFormItem("Supply", container.NewHScroll(supply)),
			widget.NewFormItem("Decimals", container.NewHScroll(decimals)),
		)
	)
	scroll.SetMinSize(fyne.NewSize(200, 0))
	dialog.ShowCustomConfirm("New Token", "OK", "Cancel", content, func(ok bool) {
		if ok {
			if name.Text == "" {
				dialog.ShowError(errors.New("Name is empty"), win)
				return
			}
			decimals, err := strconv.ParseUint(decimals.Text, 10, 8)
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			supply, err := tcm.amountFromString(supply.Text, byte(decimals))
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if supply.BitLen() > 16*8 {
				dialog.ShowError(errors.New("Supply is too big"), win)
				return
			}
			a, err := tl.getAccount()
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			prog := dialog.NewProgressInfinite(name.Text, "Creating token...", win)
			prog.Show()
			token, err := tcm.createToken(nil, a, name.Text, supply, byte(decimals))
			prog.Hide()
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if err = tcm.save(); err != nil {
				dialog.ShowError(err, win)
			}
			tl.list.Refresh()
			showSuccessDialog(win, token.Hash())
		}
	}, win)
}

func (tl *tokenList) showAddTokenDialog(win fyne.Window) {
	var (
		hash    = widget.NewEntry()
		scroll  = container.NewHScroll(hash)
		content = widget.NewForm(widget.NewFormItem("Hash", scroll))
	)
	scroll.SetMinSize(fyne.NewSize(600, 0))
	dialog.ShowCustomConfirm("Add Existing Token", "OK", "Cancel", content, func(ok bool) {
		if ok {
			hash, err := hex.DecodeString(hash.Text)
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			prog := dialog.NewProgressInfinite("Add Existing Token", "Loading token...", win)
			prog.Show()
			token, err := tcm.fetchToken(hash)
			prog.Hide()
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if err = tcm.save(); err != nil {
				dialog.ShowError(err, win)
			}
			tl.list.Refresh()
			dialog.ShowInformation("Add Existing Token", "Added "+token.Name(), win)
		}
	}, win)
}

func (tl *tokenList) showTransferDialog(win fyne.Window) {
	var (
		account = widget.NewEntry()
		amount  = widget.NewEntry()
		max     = widget.NewButton("Max", func() {
			amount.SetText(tcm.amountToString(tcm.getBalance(tl.selectedToken, tl.ai.address), tl.selectedToken.Decimals()))
		})
		scroll  = container.NewHScroll(amount)
		content = widget.NewForm(
			widget.NewFormItem("Recipient", container.NewHScroll(account)),
			widget.NewFormItem("Amount", container.NewHBox(scroll, max)),
		)
	)
	scroll.SetMinSize(fyne.NewSize(500, 0))
	account.SetPlaceHolder("Address to send to")
	amount.SetPlaceHolder("Amount of " + tl.selectedToken.Name() + " to send")
	dialog.ShowCustomConfirm("Send from "+tl.ai.address, "OK", "Cancel", content, func(ok bool) {
		if ok {
			amount, err := tcm.amountFromString(amount.Text, tl.selectedToken.Decimals())
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			a, err := tl.getAccount()
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			prog := dialog.NewProgressInfinite(tl.selectedToken.Name(), "Transferring token...", win)
			prog.Show()
			hash, err := tcm.transferToken(tl.selectedToken, a, account.Text, amount)
			prog.Hide()
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			tl.list.Refresh()
			showSuccessDialog(win, hash)
		}
	}, win)
}
