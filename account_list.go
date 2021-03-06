package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"fyne.io/fyne"
	"fyne.io/fyne/container"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	"github.com/hectorchu/gonano/rpc"
	"github.com/hectorchu/gonano/util"
)

type accountList struct {
	m                         sync.Mutex
	widget                    fyne.CanvasObject
	list                      *widget.List
	addButton, removeButton   *widget.Button
	sendButton, receiveButton *widget.Button
	receiveAllButton          *widget.Button
	changeRepButton           *widget.Button
	tokensButton              *widget.Button
	toggleThemeButton         *widget.Button
	wl                        *walletList
	wi                        *walletInfo
	selectedAccount           *accountInfo
}

func newAccountList(win fyne.Window) (al *accountList) {
	al = &accountList{
		list: widget.NewList(
			func() int {
				if al.wi == nil {
					return 0
				}
				return len(al.wi.accountsList)
			},
			func() fyne.CanvasObject {
				return fyne.NewContainerWithLayout(
					newHBoxLayout([]int{600}), newCopyableLabel(win, ""), newCopyableLabel(win, ""),
				)
			},
			func(id widget.ListItemID, item fyne.CanvasObject) {
				if id >= len(al.wi.accountsList) {
					return
				}
				ai := al.wi.accountsList[id]
				al.m.Lock()
				var balance string
				if ai.balance.Raw != nil {
					balance = ai.balance.String()
				}
				if ai.pending.Raw != nil && ai.pending.Raw.Sign() > 0 {
					balance += fmt.Sprintf(" (+ %s)", ai.pending)
				}
				al.m.Unlock()
				getLabel := func(i int) *contextMenuLabel {
					return item.(*fyne.Container).Objects[i].(*contextMenuLabel)
				}
				getLabel(0).SetText(ai.address)
				getLabel(1).SetText(balance)
				getLabel(0).tapped = func() { al.list.Select(id) }
				getLabel(1).tapped = func() { al.list.Select(id) }
			},
		),
		addButton: widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
			if err := al.addAccount(); err != nil {
				dialog.ShowError(err, win)
			}
		}),
		removeButton: widget.NewButtonWithIcon("Remove", theme.ContentRemoveIcon(), func() {
			if err := al.removeAccount(); err != nil {
				dialog.ShowError(err, win)
			}
		}),
		sendButton: widget.NewButtonWithIcon("Send", theme.MailForwardIcon(), func() {
			al.showSendDialog(win)
		}),
		receiveButton: widget.NewButtonWithIcon("Receive", theme.MailReplyIcon(), func() {
			if err := al.receive(win); err != nil {
				dialog.ShowError(err, win)
			}
		}),
		receiveAllButton: widget.NewButtonWithIcon("Receive All", theme.MailReplyAllIcon(), func() {
			if err := al.receiveAll(win); err != nil {
				dialog.ShowError(err, win)
			}
		}),
		changeRepButton: widget.NewButtonWithIcon("Change Rep",
			theme.NewThemedResource(resourceUserTieSvg, nil), func() {
				al.showChangeRepDialog(win)
			},
		),
		tokensButton: widget.NewButtonWithIcon("Tokens",
			theme.NewThemedResource(resourceTagsSvg, nil), func() {
				if al.wi == nil || al.selectedAccount == nil {
					return
				}
				newTokenList(al.wi, al.selectedAccount)
			},
		),
		toggleThemeButton: widget.NewButtonWithIcon("", toggleThemeResource(), func() {
			toggleTheme()
			al.toggleThemeButton.SetIcon(toggleThemeResource())
		}),
	}
	al.widget = container.NewBorder(
		widget.NewLabel("Accounts:"),
		widget.NewHBox(
			al.addButton, al.removeButton, al.sendButton,
			al.receiveButton, al.receiveAllButton, al.changeRepButton,
			al.tokensButton, layout.NewSpacer(), al.toggleThemeButton,
		),
		nil, nil, al.list,
	)
	al.list.OnSelected = func(id widget.ListItemID) { al.setAccount(al.wi.accountsList[id]) }
	al.list.OnUnselected = func(id widget.ListItemID) { al.setAccount(nil) }
	al.setWallet(nil)
	wsClient.subscribe(func(block *rpc.Block) {
		al.m.Lock()
		if al.wi != nil {
			if al.wi.updateBalance(block.Account) {
				defer al.list.Refresh()
			}
			if _, ok := al.wi.Accounts[block.LinkAsAccount]; ok {
				go func() {
					al.m.Lock()
					if al.wi != nil {
						if al.wi.updateBalance(block.LinkAsAccount) {
							defer al.list.Refresh()
						}
					}
					al.m.Unlock()
				}()
			}
		}
		al.m.Unlock()
	})
	return
}

func toggleThemeResource() fyne.Resource {
	res := resourceSunSvg
	if lightTheme {
		res = resourceMoonSvg
	}
	return theme.NewThemedResource(res, nil)
}

func (al *accountList) setWallet(wi *walletInfo) {
	al.m.Lock()
	al.wi = wi
	al.m.Unlock()
	if wi == nil {
		al.addButton.Disable()
		al.removeButton.Disable()
		al.sendButton.Disable()
		al.receiveButton.Disable()
		al.receiveAllButton.Disable()
		al.changeRepButton.Disable()
		al.tokensButton.Disable()
		al.setAccount(nil)
	} else {
		al.addButton.Enable()
		if len(wi.accountsList) > 0 {
			al.receiveAllButton.Enable()
		} else {
			al.receiveAllButton.Disable()
		}
	}
	al.list.Unselect(0)
	al.list.Refresh()
	go func() {
		al.m.Lock()
		if al.wi != nil {
			al.wi.getBalances()
		}
		al.m.Unlock()
		al.list.Refresh()
	}()
}

func (al *accountList) setAccount(ai *accountInfo) {
	al.selectedAccount = ai
	if ai == nil {
		al.removeButton.Disable()
		al.sendButton.Disable()
		al.receiveButton.Disable()
		al.changeRepButton.Disable()
		al.tokensButton.Disable()
	} else {
		al.removeButton.Enable()
		al.sendButton.Enable()
		al.receiveButton.Enable()
		al.changeRepButton.Enable()
		al.tokensButton.Enable()
	}
}

func (al *accountList) addAccount() (err error) {
	if al.wi == nil {
		return
	}
	al.m.Lock()
	err = al.wi.addAccount()
	al.m.Unlock()
	if err != nil {
		return
	}
	if len(al.wi.accountsList) > 0 {
		al.receiveAllButton.Enable()
	}
	al.list.Refresh()
	return al.wl.saveWallet(al.wi)
}

func (al *accountList) removeAccount() (err error) {
	if al.wi == nil || al.selectedAccount == nil {
		return
	}
	i := al.wi.indexOf(al.selectedAccount)
	al.m.Lock()
	al.wi.removeAccount(al.selectedAccount)
	al.m.Unlock()
	switch len(al.wi.accountsList) {
	case 0:
		al.list.Unselect(0)
		al.receiveAllButton.Disable()
	case i:
		al.list.Select(i - 1)
	default:
		al.setAccount(al.wi.accountsList[i])
	}
	al.list.Refresh()
	return al.wl.saveWallet(al.wi)
}

func (al *accountList) showSendDialog(win fyne.Window) {
	var (
		account = widget.NewEntry()
		amount  = widget.NewEntry()
		max     = widget.NewButton("Max", func() {
			al.m.Lock()
			if al.selectedAccount.balance.Raw != nil {
				amount.SetText(al.selectedAccount.balance.String())
			}
			al.m.Unlock()
		})
		paymentURL = widget.NewEntry()
		scroll     = container.NewHScroll(amount)
		content    = widget.NewForm(
			widget.NewFormItem("Recipient", container.NewHScroll(account)),
			widget.NewFormItem("Amount", container.NewHBox(scroll, max)),
			widget.NewFormItem("Payment URL", container.NewHScroll(paymentURL)),
		)
	)
	scroll.SetMinSize(fyne.NewSize(500, 0))
	account.SetPlaceHolder("Address to send to")
	amount.SetPlaceHolder("Amount of NANO to send")
	paymentURL.SetPlaceHolder("URL to send block to (leave blank to send to network)")
	dialog.ShowCustomConfirm(
		"Send from "+al.selectedAccount.address, "OK", "Cancel", content, func(ok bool) {
			if ok {
				if err := al.send(win, account.Text, amount.Text, paymentURL.Text); err != nil {
					dialog.ShowError(err, win)
				}
			}
		}, win,
	)
}

func (al *accountList) send(win fyne.Window, account, amount, paymentURL string) (err error) {
	n, err := util.NanoAmountFromString(amount)
	if err != nil {
		return
	}
	a, err := al.wi.w.NewAccount(&al.selectedAccount.Index)
	if err != nil {
		return
	}
	if a.Address() != al.selectedAccount.address {
		return errors.New("Address mismatch")
	}
	var hash rpc.BlockHash
	prog := dialog.NewProgressInfinite(al.wi.Label, "Generating block...", win)
	prog.Show()
	if paymentURL == "" {
		hash, err = a.Send(account, n.Raw)
		prog.Hide()
		if err != nil {
			return
		}
	} else {
		block, err := a.SendBlock(account, n.Raw)
		prog.Hide()
		if err != nil {
			return err
		}
		if hash, err = block.Hash(); err != nil {
			return err
		}
		prog = dialog.NewProgressInfinite(al.wi.Label, "Waiting for confirmation...", win)
		prog.Show()
		err = sendToPaymentURL(paymentURL, block)
		prog.Hide()
		if err != nil {
			return err
		}
	}
	showSuccessDialog(win, hash)
	return
}

func sendToPaymentURL(paymentURL string, block *rpc.Block) (err error) {
	var (
		buf  bytes.Buffer
		resp *http.Response
	)
	if err = json.NewEncoder(&buf).Encode(block); err != nil {
		return
	}
	if resp, err = http.Post(paymentURL, "application/json", &buf); err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf.Reset()
		io.Copy(&buf, resp.Body)
		return errors.New(buf.String())
	}
	return
}

func showSuccessDialog(win fyne.Window, hash rpc.BlockHash) {
	var (
		url, _    = url.Parse("https://nanolooker.com/block/" + hash.String())
		label     = widget.NewLabel("Sent with block hash")
		hyperlink = widget.NewHyperlink(hash.String(), url)
	)
	dialog.ShowCustom("Success", "OK", container.NewHBox(label, hyperlink), win)
}

func (al *accountList) receive(win fyne.Window) (err error) {
	a, err := al.wi.w.NewAccount(&al.selectedAccount.Index)
	if err != nil {
		return
	}
	if a.Address() != al.selectedAccount.address {
		return errors.New("Address mismatch")
	}
	prog := dialog.NewProgressInfinite(al.wi.Label, "Receiving pending amounts...", win)
	prog.Show()
	err = a.ReceivePendings()
	prog.Hide()
	return
}

func (al *accountList) receiveAll(win fyne.Window) (err error) {
	for _, ai := range al.wi.Accounts {
		a, err := al.wi.w.NewAccount(&ai.Index)
		if err != nil {
			return err
		}
		if a.Address() != ai.address {
			return errors.New("Address mismatch")
		}
	}
	prog := dialog.NewProgressInfinite(al.wi.Label, "Receiving pending amounts...", win)
	prog.Show()
	err = al.wi.w.ReceivePendings()
	prog.Hide()
	return
}

func (al *accountList) showChangeRepDialog(win fyne.Window) {
	var (
		rpcClient     = rpc.Client{URL: rpcURL}
		currentRep, _ = rpcClient.AccountRepresentative(al.selectedAccount.address)
		label         = newCopyableLabel(win, currentRep)
		account       = widget.NewEntry()
		scroll        = container.NewHScroll(account)
		content       = widget.NewForm(
			widget.NewFormItem("Current representative", label),
			widget.NewFormItem("New representative", scroll),
		)
	)
	scroll.SetMinSize(fyne.NewSize(580, 0))
	account.SetPlaceHolder("Representative address")
	dialog.ShowCustomConfirm("Change representative", "OK", "Cancel", content, func(ok bool) {
		if ok {
			if err := al.changeRep(win, account.Text); err != nil {
				dialog.ShowError(err, win)
			}
		}
	}, win)
}

func (al *accountList) changeRep(win fyne.Window, account string) (err error) {
	a, err := al.wi.w.NewAccount(&al.selectedAccount.Index)
	if err != nil {
		return
	}
	if a.Address() != al.selectedAccount.address {
		return errors.New("Address mismatch")
	}
	prog := dialog.NewProgressInfinite(al.wi.Label, "Generating block...", win)
	prog.Show()
	hash, err := a.ChangeRep(account)
	prog.Hide()
	if err != nil {
		return
	}
	showSuccessDialog(win, hash)
	return
}
