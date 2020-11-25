package main

import (
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
	win := f.NewWindow("Gonano v0.1.0")
	if err := initConfig(); err != nil {
		dialog.ShowError(err, win)
	}
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
	for _, url := range []string{
		"https://mynano.ninja/api/node",
		"https://nault.nanos.cc/proxy",
		"https://voxpopuli.network/api",
		"https://vault.nanocrawler.cc/api/node-api",
	} {
		rpcClient := rpc.Client{URL: url}
		_, _, unchecked, err := rpcClient.BlockCount()
		if err == nil && unchecked < 1000 {
			rpcURL = url
			break
		}
	}
}