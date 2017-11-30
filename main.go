package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/voloshink/dggchat"
)

type config struct {
	DGGKey        string   `json:"dgg_key"`
	CustomURL     string   `json:"custom_url"`
	Username      string   `json:"username"`
	Highlighted   []string `json:"highlighted"`
	ShowJoinLeave bool     `json:"showjoinleave"`
}

var configFile string

func init() {
	flag.StringVar(&configFile, "config", "config.json", "location of config file to be used")
}

type mute struct {
	dggchat.Mute
}

type unmute struct {
	dggchat.Mute
}

type ban struct {
	dggchat.Ban
}

type unban struct {
	dggchat.Ban
}

type joinAction struct {
	dggchat.RoomAction
}

type quitAction struct {
	dggchat.RoomAction
}

func main() {

	flag.Parse()

	file, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalln(err)
	}

	var config config
	err = json.Unmarshal(file, &config)
	if err != nil {
		log.Println("malformed configuration file:")
		log.Fatalln(err)
	}

	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Fatalln(err)
	}
	defer g.Close()

	g.SetManagerFunc(layout)
	g.Mouse = false

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		log.Fatalln(err)
	}

	chat, err := newChat(&config, g)
	if err != nil {
		log.Println(err)
		return
	}

	if err := g.SetKeybinding("input", gocui.KeyArrowUp, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		historyUp(g, v, chat)
		return nil
	}); err != nil {
		log.Panicln(err)
	}

	if err := g.SetKeybinding("input", gocui.KeyArrowDown, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		historyDown(g, v, chat)
		return nil
	}); err != nil {
		log.Panicln(err)
	}

	err = g.SetKeybinding("input", gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {

		if v.Buffer() == "" {
			return nil
		}

		chat.handleInput(strings.TrimSpace(v.Buffer()), g)
		g.Update(func(g *gocui.Gui) error {
			v.Clear()
			v.SetCursor(0, 0)
			v.SetOrigin(0, 0)
			return nil
		})

		return nil
	})

	if err != nil {
		log.Panicln(err)
	}

	events := make(chan interface{}, 100)

	chat.Session.AddMessageHandler(func(m dggchat.Message, s *dggchat.Session) {
		events <- m
	})
	chat.Session.AddErrorHandler(func(e string, s *dggchat.Session) {
		events <- fmt.Errorf(e)
	})
	chat.Session.AddMuteHandler(func(m dggchat.Mute, s *dggchat.Session) {
		events <- mute{m}
	})
	chat.Session.AddUnmuteHandler(func(m dggchat.Mute, s *dggchat.Session) {
		events <- unmute{m}
	})
	chat.Session.AddBanHandler(func(b dggchat.Ban, s *dggchat.Session) {
		events <- ban{b}
	})
	chat.Session.AddUnbanHandler(func(b dggchat.Ban, s *dggchat.Session) {
		events <- unban{b}
	})
	chat.Session.AddJoinHandler(func(r dggchat.RoomAction, s *dggchat.Session) {
		events <- joinAction{r}
	})
	chat.Session.AddQuitHandler(func(r dggchat.RoomAction, s *dggchat.Session) {
		events <- quitAction{r}
	})
	chat.Session.AddSubOnlyHandler(func(so dggchat.SubOnly, s *dggchat.Session) {
		events <- so
	})
	chat.Session.AddBroadcastHandler(func(b dggchat.Broadcast, s *dggchat.Session) {
		events <- b
	})
	chat.Session.AddPingHandler(func(p dggchat.Ping, s *dggchat.Session) {
		events <- p
	})

	err = chat.Session.Open()
	if err != nil {
		log.Println(err)
		return
	}
	defer chat.Session.Close()

	// TODO need to wait for lib to receive first NAMES message to be properly "initialized"
	// maybe add a handler for this instead
	for {
		if len(chat.Session.GetUsers()) != 0 {
			break
		}
		time.Sleep(time.Millisecond * 300)
	}

	chat.renderUsers(chat.Session.GetUsers())

	go func() { //TODO
		for {
			event := <-events
			switch event.(type) {
			case dggchat.Message:
				chat.renderMessage(event.(dggchat.Message))
			case error:
				chat.renderError(event.(error).Error())
			case dggchat.Ping:
				_ = event.(dggchat.Ping).Timestamp //TODO
			case mute:
				chat.renderMute(event.(mute).Mute)
			case unmute:
				chat.renderUnmute(event.(unmute).Mute)
			case ban:
				chat.renderBan(event.(ban).Ban)
			case unban:
				chat.renderUnban(event.(unban).Ban)
			case joinAction:
				if chat.config.ShowJoinLeave {
					chat.renderJoin(event.(joinAction).RoomAction)
				}
				chat.renderUsers(chat.Session.GetUsers())
			case quitAction:
				if chat.config.ShowJoinLeave {
					chat.renderQuit(event.(quitAction).RoomAction)
				}
				chat.renderUsers(chat.Session.GetUsers())
			case dggchat.SubOnly:
				chat.renderSubOnly(event.(dggchat.SubOnly))
			case dggchat.Broadcast:
				chat.renderBroadcast(event.(dggchat.Broadcast))
			}
		}
	}()

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Fatalln(err)
	}

}
