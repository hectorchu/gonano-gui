package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne"
	"fyne.io/fyne/container"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	"github.com/spf13/viper"
	"github.com/tyler-smith/go-bip39"
)

type walletList struct {
	widget         fyne.CanvasObject
	list           *widget.List
	addButton      *contextMenuButton
	removeButton   *widget.Button
	wallets        []*walletInfo
	selectedWallet *walletInfo
	al             *accountList
}

func newWalletList(win fyne.Window, al *accountList) (wl *walletList) {
	wl = &walletList{
		list: widget.NewList(
			func() int { return len(wl.wallets) },
			func() fyne.CanvasObject { return newContextMenuLabel("", nil) },
			func(id widget.ListItemID, item fyne.CanvasObject) {
				if id >= len(wl.wallets) {
					return
				}
				wi := wl.wallets[id]
				l := item.(*contextMenuLabel)
				l.SetText(wi.Label)
				l.tapped = func() { wl.list.Select(id) }
				l.menu = fyne.NewMenu("", fyne.NewMenuItem("Rename", func() {
					wl.showRenameDialog(win, wi)
				}))
			},
		),
		addButton: newContextMenuButton("Add", theme.ContentAddIcon(), fyne.NewMenu("",
			fyne.NewMenuItem("Nano seed", func() { wl.showSeedWalletDialog(win) }),
			fyne.NewMenuItem("BIP39 mnemonic", func() { wl.showBip39WalletDialog(win) }),
			fyne.NewMenuItem("Ledger HW wallet", func() {
				if err := wl.newLedgerWallet(win); err != nil {
					dialog.ShowError(err, win)
				}
			}),
		)),
		removeButton: widget.NewButtonWithIcon("Remove", theme.ContentRemoveIcon(), func() {
			dialog.ShowConfirm(
				"Are you sure?", "You will only be able to restore from seed/mnemonic.", func(ok bool) {
					if ok {
						if err := wl.removeWallet(wl.selectedWallet); err != nil {
							dialog.ShowError(err, win)
						}
					}
				}, win,
			)
		}),
		al: al,
	}
	wl.widget = container.NewBorder(
		widget.NewLabel("Wallets:"),
		widget.NewHBox(wl.addButton, wl.removeButton),
		nil, nil, wl.list,
	)
	wl.list.OnSelected = func(id widget.ListItemID) { wl.setWallet(win, wl.wallets[id]) }
	wl.list.OnUnselected = func(id widget.ListItemID) { wl.setWallet(win, nil) }
	wl.setWallet(win, nil)
	wl.initWallets()
	return
}

func (wl *walletList) initWallets() {
	v := viper.GetStringMap("wallets")
	for i := len(v) - 1; i >= 0; i-- {
		if _, ok := v[strconv.Itoa(i)].(string); !ok {
			wl.wallets = make([]*walletInfo, i+1)
			break
		}
	}
	for i := range wl.wallets {
		key := func(s string) string {
			return fmt.Sprintf("wallets.%d.%s", i, s)
		}
		wl.wallets[i] = &walletInfo{
			Label:    viper.GetString(key("label")),
			Seed:     viper.GetString(key("seed")),
			Salt:     viper.GetString(key("salt")),
			IsBip39:  viper.GetBool(key("isBip39")),
			IsLedger: viper.GetBool(key("isLedger")),
			Accounts: make(map[string]*accountInfo),
		}
		for k, v := range viper.GetStringMap(key("accounts")) {
			v := v.(map[string]interface{})
			wl.wallets[i].Accounts[k] = &accountInfo{
				address: k,
				Index:   uint32(v["index"].(int)),
			}
		}
	}
}

func (wl *walletList) saveWallet(wi *walletInfo) (err error) {
	for i := range wl.wallets {
		if wi == wl.wallets[i] {
			viper.Set(fmt.Sprintf("wallets.%d", i), wi)
			err = viper.WriteConfig()
			break
		}
	}
	return
}

func (wl *walletList) setWallet(win fyne.Window, wi *walletInfo) {
	if wi != nil {
		init := func(password string) (err error) {
			prog := dialog.NewProgressInfinite(wi.Label, "Loading wallet...", win)
			prog.Show()
			err = wi.init(password)
			prog.Hide()
			if err != nil {
				return
			}
			wl.removeButton.Enable()
			wl.selectedWallet = wi
			wl.al.setWallet(wi)
			return
		}
		if err := init(""); err != nil {
			showPasswordDialog(win, wi.Label, init)
		}
	} else {
		wl.removeButton.Disable()
		wl.selectedWallet = nil
		wl.al.setWallet(nil)
	}
}

func (wl *walletList) showRenameDialog(win fyne.Window, wi *walletInfo) {
	var (
		label   = widget.NewEntry()
		scroll  = container.NewHScroll(label)
		content = widget.NewForm(widget.NewFormItem("New label", scroll))
	)
	label.SetText(wi.Label)
	scroll.SetMinSize(fyne.NewSize(200, 0))
	dialog.ShowCustomConfirm("Rename wallet", "OK", "Cancel", content, func(ok bool) {
		if ok {
			wi.Label = label.Text
			wl.list.Refresh()
			if err := wl.saveWallet(wi); err != nil {
				dialog.ShowError(err, win)
			}
		}
	}, win)
}

func (wl *walletList) removeWallet(wi *walletInfo) (err error) {
	for i := range wl.wallets {
		if wi == wl.wallets[i] {
			wl.wallets = append(wl.wallets[:i], wl.wallets[i+1:]...)
			wl.list.Unselect(i)
			wl.list.Refresh()
			for ; i < len(wl.wallets); i++ {
				viper.Set(fmt.Sprintf("wallets.%d", i), wl.wallets[i])
			}
			viper.Set(fmt.Sprintf("wallets.%d", i), "")
			err = viper.WriteConfig()
			break
		}
	}
	return
}

func showPasswordDialog(win fyne.Window, title string, callback func(string) error) {
	var (
		password = widget.NewPasswordEntry()
		scroll   = container.NewHScroll(password)
		content  = widget.NewForm(widget.NewFormItem("Password", scroll))
	)
	scroll.SetMinSize(fyne.NewSize(400, 0))
	dialog.ShowCustomConfirm(title, "OK", "Cancel", content, func(ok bool) {
		if ok {
			if err := callback(password.Text); err != nil {
				dialog.ShowError(err, win)
			}
		}
	}, win)
}

func (wl *walletList) showSeedWalletDialog(win fyne.Window) {
	var (
		label     = widget.NewEntry()
		seed      = widget.NewEntry()
		password  = widget.NewPasswordEntry()
		password2 = widget.NewPasswordEntry()
		scroll    = container.NewHScroll(label)
		content   = widget.NewForm(
			widget.NewFormItem("Label", scroll),
			widget.NewFormItem("Seed", container.NewHScroll(seed)),
			widget.NewFormItem("Password", container.NewHScroll(password)),
			widget.NewFormItem("Confirm password", container.NewHScroll(password2)),
		)
	)
	scroll.SetMinSize(fyne.NewSize(400, 0))
	label.SetText(fmt.Sprintf("Wallet #%d", len(wl.wallets)+1))
	seed.SetPlaceHolder("64 character, hexadecimal string (0-9A-F)")
	dialog.ShowCustomConfirm("New Seed Wallet", "OK", "Cancel", content, func(ok bool) {
		if ok {
			if password.Text != password2.Text {
				dialog.ShowError(errors.New("Passwords don't match"), win)
				return
			}
			if err := wl.newSeedWallet(win, label.Text, seed.Text, password.Text); err != nil {
				dialog.ShowError(err, win)
			}
		}
	}, win)
}

func (wl *walletList) newSeedWallet(win fyne.Window, label, seed, password string) (err error) {
	if len(seed) != 64 {
		err = errors.New("Seed must be 64 characters")
		return
	}
	prog := dialog.NewProgressInfinite(label, "Generating key...", win)
	prog.Show()
	key, salt, err := deriveKey(password, nil)
	prog.Hide()
	if err != nil {
		return
	}
	wi := &walletInfo{
		Label: label,
		Salt:  hex.EncodeToString(salt),
	}
	var seed2, enc []byte
	if seed2, err = hex.DecodeString(seed); err != nil {
		return
	}
	if enc, err = encrypt(seed2, key); err != nil {
		return
	}
	wi.Seed = hex.EncodeToString(enc)
	if err = wi.initSeed(seed2); err != nil {
		return
	}
	if err = wi.initAccounts(win); err != nil {
		return
	}
	wl.wallets = append(wl.wallets, wi)
	wl.list.Refresh()
	return wl.saveWallet(wi)
}

func (wl *walletList) showBip39WalletDialog(win fyne.Window) {
	var (
		label     = widget.NewEntry()
		mnemonic  = widget.NewEntry()
		password  = widget.NewPasswordEntry()
		password2 = widget.NewPasswordEntry()
		scroll    = container.NewHScroll(label)
		content   = widget.NewForm(
			widget.NewFormItem("Label", scroll),
			widget.NewFormItem("Mnemonic", container.NewHScroll(mnemonic)),
			widget.NewFormItem("Password", container.NewHScroll(password)),
			widget.NewFormItem("Confirm password", container.NewHScroll(password2)),
		)
	)
	scroll.SetMinSize(fyne.NewSize(400, 0))
	label.SetText(fmt.Sprintf("BIP39 Wallet #%d", len(wl.wallets)+1))
	mnemonic.SetPlaceHolder("24 word secret (leave blank for random)")
	dialog.ShowCustomConfirm("New BIP39 Wallet", "OK", "Cancel", content, func(ok bool) {
		if ok {
			if password.Text != password2.Text {
				dialog.ShowError(errors.New("Passwords don't match"), win)
				return
			}
			if err := wl.newBip39Wallet(win, label.Text, mnemonic.Text, password.Text); err != nil {
				dialog.ShowError(err, win)
			}
		}
	}, win)
}

func (wl *walletList) newBip39Wallet(win fyne.Window, label, mnemonic, password string) (err error) {
	prog := dialog.NewProgressInfinite(label, "Generating key...", win)
	prog.Show()
	key, salt, err := deriveKey(password, nil)
	prog.Hide()
	if err != nil {
		return
	}
	wi := &walletInfo{
		Label:   label,
		Salt:    hex.EncodeToString(salt),
		IsBip39: true,
	}
	var entropy []byte
	if mnemonic == "" {
		if entropy, err = bip39.NewEntropy(256); err != nil {
			return
		}
		if mnemonic, err = bip39.NewMnemonic(entropy); err != nil {
			return
		}
		words := strings.Split(mnemonic, " ")
		mnemonic = strings.Join(words[:12], " ") + "\n" + strings.Join(words[12:], " ")
		dialog.ShowInformation("Your secret words are:", mnemonic, win)
	} else {
		if entropy, err = bip39.EntropyFromMnemonic(mnemonic); err != nil {
			return
		}
	}
	enc, err := encrypt(entropy, key)
	if err != nil {
		return
	}
	wi.Seed = hex.EncodeToString(enc)
	if err = wi.initBip39(entropy, password); err != nil {
		return
	}
	if err = wi.initAccounts(win); err != nil {
		return
	}
	wl.wallets = append(wl.wallets, wi)
	wl.list.Refresh()
	return wl.saveWallet(wi)
}

func (wl *walletList) newLedgerWallet(win fyne.Window) (err error) {
	wi := &walletInfo{
		Label:    fmt.Sprintf("Ledger Wallet #%d", len(wl.wallets)+1),
		IsLedger: true,
	}
	if err = wi.initLedger(); err != nil {
		return
	}
	if err = wi.initAccounts(win); err != nil {
		return
	}
	wl.wallets = append(wl.wallets, wi)
	wl.list.Refresh()
	return wl.saveWallet(wi)
}
