package main

import (
	"sync"

	"github.com/hectorchu/gonano/rpc"
	"github.com/hectorchu/gonano/websocket"
)

type wsClientType struct {
	m   sync.Mutex
	key int
	sub []wsClientSub
}

type wsClientSub struct {
	key int
	f   func(*rpc.Block)
}

var wsClient = wsClientType{}

func init() {
	for _, url := range []string{
		"wss://ws.mynano.ninja",
		"wss://vox.nanos.cc/websocket",
	} {
		ws := &websocket.Client{URL: url}
		if ws.Connect() == nil {
			go wsClient.loop(ws)
			return
		}
	}
}

func (c *wsClientType) subscribe(f func(*rpc.Block)) (key int) {
	c.m.Lock()
	key = c.key
	c.key++
	c.sub = append(c.sub, wsClientSub{key: key, f: f})
	c.m.Unlock()
	return
}

func (c *wsClientType) unsubscribe(key int) {
	c.m.Lock()
	for i, sub := range c.sub {
		if sub.key == key {
			c.sub = append(c.sub[:i], c.sub[i+1:]...)
			break
		}
	}
	c.m.Unlock()
}

func (c *wsClientType) loop(ws *websocket.Client) {
	for {
		m := <-ws.Messages
		c.m.Lock()
		switch m := m.(type) {
		case *websocket.Confirmation:
			for _, sub := range c.sub {
				sub.f(m.Block)
			}
		}
		c.m.Unlock()
	}
}
