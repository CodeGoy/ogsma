package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type GUI struct {
	tabs            bool
	app             fyne.App
	window          fyne.Window
	chatOutput      map[string]*widget.RichText
	scrollContainer map[string]*container.Scroll
	client          *Client
	enc             *Encryption
	serverKey       string
	notification    bool
}

func (g *GUI) loginWindow() {
	g.window.SetTitle("OGSMA")
	messageLabel := widget.NewLabel("")
	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder("Password")
	login := func() {
		g.enc.password = passEntry.Text
		if err := g.enc.loadKeys(); err != nil {
			messageLabel.SetText(fmt.Sprintf("Invalid Password: %v", err))
			return
		}
		g.client.ID = g.enc.keys.ID
		if err := g.client.Connect(); err != nil {
			messageLabel.SetText(fmt.Sprintf("Unable to contact server:\n %v", err))
			return
		}
		for _, contact := range g.enc.keys.Contacts {
			g.chatOutput[contact.ID] = widget.NewRichText()
			g.chatOutput[contact.ID].Wrapping = fyne.TextWrapWord
			g.scrollContainer[contact.ID] = container.NewVScroll(g.chatOutput[contact.ID])
		}
		g.contactsWindow()
	}
	passEntry.OnSubmitted = func(s string) {
		login()
	}
	////////////
	// passEntry.SetText("password1234!")
	////////////
	loginButton := widget.NewButton("Login", login)
	content := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("Login")),
		passEntry,
		loginButton,
		messageLabel,
		widget.NewCheck("tabs", func(value bool) {
			log.Println("tabs option set to", value)
			g.tabs = value
		}),
		widget.NewCheck("notification", func(value bool) {
			log.Println("notification option set to", value)
			g.notification = value
		}),
	)
	g.window.SetContent(content)
}

func (g *GUI) contactsWindow() {
	g.window.SetTitle("Contacts")
	if g.tabs {
		tabs := container.NewAppTabs()
		for _, contact := range g.enc.keys.Contacts {
			tabs.Append(container.NewTabItem(contact.Username, g.chatTab(contact)))
		}
		g.window.SetContent(tabs)
	} else {
		content := container.NewVBox(widget.NewLabel("Contacts"))
		for _, contact := range g.enc.keys.Contacts {
			content.Add(widget.NewButton(contact.Username, func() {
				g.chatWindow(contact)
			}))
			content.Add(widget.NewSeparator())
		}
		g.window.SetContent(content)
	}
}

type Msg struct {
	ID        string    `json:"id"`
	Message   []byte    `json:"msg"`
	TimeStamp time.Time `json:"timestamp"`
	FromID    string    `json:"from"`
}

func (g *GUI) sendMessage(contact *Contact, msg string) {
	targetPublicKeyEncryptedBytes, err := g.enc.publicEncrypt([]byte(msg), contact.PublicKey)
	if err != nil {
		log.Println(err)
		return
	}
	jm, err := json.Marshal(&Msg{
		ID:        contact.ID,
		TimeStamp: time.Now(),
		Message:   targetPublicKeyEncryptedBytes,
		FromID:    g.client.ID,
	})
	if err != nil {
		log.Printf("error: json marshal: %v", err)
		return
	}
	encryptedMessage, err := g.enc.passwordEncrypt(jm, g.serverKey)
	if err != nil {
		log.Printf("error: json encrypt: %v", err)
		return
	}
	if err := g.client.SendMsg(encryptedMessage); err != nil {
		log.Println(err)
		g.appendText("Error:", err.Error(), contact.ID, true)
	}
	g.appendText(g.enc.keys.Username, msg, contact.ID, true)
}

func (g *GUI) chatWindow(contact *Contact) {
	g.window.SetTitle("messaging")
	msgEntry := widget.NewEntry()
	msgEntry.OnSubmitted = func(s string) {
		if len(s) > 0 {
			g.sendMessage(contact, msgEntry.Text)
			msgEntry.SetText("")
		}
	}
	backButton := widget.NewButton("back", func() {
		g.contactsWindow()
	})
	content := container.New(layout.NewBorderLayout(backButton, msgEntry, nil, nil),
		backButton,
		g.scrollContainer[contact.ID],
		msgEntry,
	)
	g.window.SetContent(content)
}

func (g *GUI) chatTab(contact *Contact) *fyne.Container {
	g.window.SetTitle("messaging")
	msgEntry := widget.NewEntry()
	msgEntry.OnSubmitted = func(s string) {
		if len(s) > 0 {
			g.sendMessage(contact, msgEntry.Text)
			msgEntry.SetText("")
		}
	}
	return container.New(layout.NewBorderLayout(nil, msgEntry, nil, nil),
		g.scrollContainer[contact.ID],
		msgEntry,
	)
}

func (g *GUI) appendText(prefix, content any, id string, self bool) {
	go func() {
		fyne.DoAndWait(func() {
			if self {
				g.chatOutput[id].AppendMarkdown(fmt.Sprintf(`* %v`, content))
			} else {
				g.chatOutput[id].AppendMarkdown(fmt.Sprintf("%v: %v", prefix, content))
			}
			g.chatOutput[id].AppendMarkdown("---")
			g.scrollContainer[id].ScrollToBottom()
			g.chatOutput[id].Refresh()
		})
	}()
}

func (g *GUI) lifecycle() {
	lifecycle := g.app.Lifecycle()
	lifecycle.SetOnStopped(func() {
		background = true
	})
	lifecycle.SetOnStarted(func() {
		background = false
	})
	lifecycle.SetOnExitedForeground(func() {
		background = true
	})
	lifecycle.SetOnEnteredForeground(func() {
		background = false
	})
}

func (g *GUI) lookupUsername(id string) (string, error) {
	for _, contact := range g.enc.keys.Contacts {
		if id == contact.ID {
			return contact.Username, nil

		}
	}
	return "", errors.New("user not found")
}

func (g *GUI) listen() {
	for {
		nm := <-g.client.MessageChan
		nms := Msg{}
		if err := json.Unmarshal(nm, &nms); err != nil {
			log.Printf("error unmarshalling message: %v", err)
		}
		decryptedMessage, err := g.enc.privateDecrypt(nms.Message)
		if err != nil {
			log.Printf("error decrypting message: %v", err)
		}
		since := time.Now().Sub(nms.TimeStamp).Round(time.Second)
		contactMessages[nms.FromID] = append(contactMessages[nms.FromID], QueueMessage{
			sent: nms.TimeStamp,
			msg:  string(decryptedMessage),
		})
		username, err := g.lookupUsername(nms.FromID)
		if err != nil {
			log.Printf("error looking up username: %v", err)
		}
		if background && g.notification {
			fyne.CurrentApp().SendNotification(&fyne.Notification{
				Title:   fmt.Sprintf("Msg from: %s", username),
				Content: fmt.Sprintf("%s", string(decryptedMessage)),
			})
		}
		if since > time.Second*5 {
			g.appendText(fmt.Sprintf("%s %v:", username, since), string(decryptedMessage), nms.FromID, false)
		} else {
			g.appendText(username, string(decryptedMessage), nms.FromID, false)
		}
	}
}
