package main

import (
	"fyne.io/fyne"
	"fyne.io/fyne/widget"
)

type contextMenuButton struct {
	widget.Button
	menu *fyne.Menu
}

func (b *contextMenuButton) Tapped(e *fyne.PointEvent) {
	widget.ShowPopUpMenuAtPosition(b.menu, fyne.CurrentApp().Driver().CanvasForObject(b), e.AbsolutePosition)
}

func newContextMenuButton(label string, icon fyne.Resource, menu *fyne.Menu) *contextMenuButton {
	b := &contextMenuButton{menu: menu}
	b.Text = label
	b.Icon = icon
	b.ExtendBaseWidget(b)
	return b
}

type contextMenuLabel struct {
	widget.Label
	menu   *fyne.Menu
	tapped func()
}

func (l *contextMenuLabel) Tapped(e *fyne.PointEvent) {
	if l.tapped != nil {
		l.tapped()
	}
}

func (l *contextMenuLabel) TappedSecondary(e *fyne.PointEvent) {
	widget.ShowPopUpMenuAtPosition(l.menu, fyne.CurrentApp().Driver().CanvasForObject(l), e.AbsolutePosition)
}

func newContextMenuLabel(label string, menu *fyne.Menu) *contextMenuLabel {
	l := &contextMenuLabel{menu: menu}
	l.Text = label
	l.ExtendBaseWidget(l)
	return l
}

func newCopyableLabel(win fyne.Window, label string) (l *contextMenuLabel) {
	return newContextMenuLabel(label, fyne.NewMenu("",
		fyne.NewMenuItem("Copy", func() { win.Clipboard().SetContent(l.Text) }),
	))
}
