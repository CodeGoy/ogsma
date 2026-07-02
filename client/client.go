package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	ID          string
	Conn        *websocket.Conn
	Addr        string
	SelfSigned  bool // SelfSigned Disables checking CA store for cert // TODO : check cert manually
	wsPath      string
	MessageChan chan []byte
	serverKey   string
	enc         *Encryption
}

func (c *Client) Connect() error {
	var err error
	c.Conn = nil
	dd := websocket.DefaultDialer
	dd.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: c.SelfSigned,
	}
	dd.HandshakeTimeout = 5 * time.Second
	c.Conn, _, err = dd.Dial(fmt.Sprintf("wss://%s/%s", c.Addr, c.wsPath), nil)
	if err != nil {
		return err
	}
	c.listener()
	mts := struct {
		ID string `json:"id"`
	}{}
	mts.ID = c.ID
	loginJson, err := json.Marshal(mts)
	if err != nil {
		return fmt.Errorf("error: json marshal: %v", err)
	}
	encryptedInitMsg, err := c.enc.passwordEncrypt(loginJson, c.serverKey)
	if err != nil {
		return fmt.Errorf("error: encrypt init: %v", err)
	}
	if err = c.Conn.WriteMessage(websocket.BinaryMessage, encryptedInitMsg); err != nil {
		return fmt.Errorf("write %v", err)
	}
	fmt.Println("connected")
	c.KeepAlive()
	return err
}

func (c *Client) listener() {
	go func() {
		for {
			mt, message, err := c.Conn.ReadMessage()
			if err != nil {
				log.Printf("failed to read: %v", err)
				break
			}
			switch mt {
			case websocket.TextMessage:
				decryptedMsg, err := c.enc.passwordDecrypt(message, c.serverKey)
				if err != nil {
					log.Printf("error: decrypt error: %v", err)
					break
				}
				c.MessageChan <- decryptedMsg
			}
		}
	}()

}

func (c *Client) KeepAlive() {
	go func() {
		for {
			time.Sleep(1 * time.Second)
			if err := c.Conn.WriteMessage(websocket.PingMessage, []byte("ping")); err == nil {
				continue
			} else {
				for {
					if err := c.Connect(); err != nil {
						log.Printf("failed to connect: %v\n", err)
						time.Sleep(5 * time.Second)
						continue
					}
					break
				}
				return
			}
		}
	}()
}

func (c *Client) SendMsg(jm []byte) error {
	return c.Conn.WriteMessage(websocket.TextMessage, jm)
}

func (c *Client) disconnect() {
	err := c.Conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.Println("write close:", err)
		return
	}
	if err := c.Conn.Close(); err != nil {
		log.Printf("error: websocket Conn close: %v", err)
	}
}
