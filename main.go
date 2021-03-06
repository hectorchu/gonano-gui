package main

import (
	"math"

	"fyne.io/fyne"
	"fyne.io/fyne/app"
	"fyne.io/fyne/container"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/theme"
	"github.com/hectorchu/gonano/rpc"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

func main() {
	f := app.New()
	f.SetIcon(resourceNanoPng)
	win := f.NewWindow("Gonano v0.1.13")
	if err := initConfig(); err != nil {
		dialog.ShowError(err, win)
	}
	go loadTokens(win)
	al := newAccountList(win)
	wl := newWalletList(win, al)
	al.wl = wl
	split := container.NewHSplit(wl.widget, al.widget)
	split.SetOffset(0)
	win.SetContent(split)
	win.Resize(fyne.NewSize(1000, 600))
	win.CenterOnScreen()
	win.ShowAndRun()
}

var lightTheme bool
var rpcURL string

func initConfig() (err error) {
	home, err := homedir.Dir()
	if err != nil {
		return
	}
	viper.AddConfigPath(home)
	viper.SetConfigName(".gonano-gui")
	viper.SetConfigType("yaml")
	if err = viper.ReadInConfig(); err != nil {
		err = viper.SafeWriteConfig()
	}
	lightTheme = viper.GetBool("lightTheme")
	setTheme()
	chooseRPC()
	return
}

func setTheme() {
	t := theme.DarkTheme()
	if lightTheme {
		t = theme.LightTheme()
	}
	fyne.CurrentApp().Settings().SetTheme(t)
}

func toggleTheme() {
	lightTheme = !lightTheme
	setTheme()
	viper.Set("lightTheme", lightTheme)
	viper.WriteConfig()
}

func chooseRPC() {
	var n uint64 = math.MaxUint64
	for _, url := range []string{
		"https://gonano.dev/rpc",
		"https://mynano.ninja/api/node",
		"https://proxy.nanos.cc/proxy",
		"https://proxy.powernode.cc/proxy",
		"https://rainstorm.city/api",
	} {
		rpcClient := rpc.Client{URL: url}
		if _, _, unchecked, err := rpcClient.BlockCount(); err == nil && unchecked < n {
			rpcURL = url
			n = unchecked
		}
	}
}

func loadTokens(win fyne.Window) {
	prog := dialog.NewProgressInfinite("Gonano", "Loading tokens...", win)
	prog.Show()
	err := tcm.load()
	prog.Hide()
	if err != nil {
		dialog.ShowError(err, win)
	}
}
