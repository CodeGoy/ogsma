package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	//go:embed config.json
	configFile []byte
)

type Config struct {
	Port      int      `json:"port"`
	Endpoint  string   `json:"endpoint"`
	CertFile  string   `json:"certFile"`
	KeyFile   string   `json:"keyFile"`
	Users     []string `json:"users"`
	ServerKey string   `json:"serverkey"`
}

type Server struct {
	endpoint     string
	websockets   map[string]*websocket.Conn
	messageQueue map[string][][]byte
	tlsPort      int
	cert         string
	key          string
	upgrader     websocket.Upgrader
	users        []string
	serverKey    string
	enc          *Encryption
	mut          sync.Mutex
}

type MessageTemplate struct {
	ID  string `json:"id"`
	Msg []byte `json:"msg"`
}

func (s *Server) oc() func(r *http.Request) bool {
	return func(r *http.Request) bool {
		return true
	}
}

func (s *Server) processMessage(msg []byte) ([]byte, error) {
	return s.enc.passwordDecrypt(msg, s.serverKey)
}

func (s *Server) removeConnection(userID string) {
	s.mut.Lock()
	delete(s.websockets, userID)
	s.mut.Unlock()
}

func (s *Server) start() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("404"))
	})
	http.HandleFunc(fmt.Sprintf("/%s", s.endpoint), func(w http.ResponseWriter, r *http.Request) {
		var currentUserID string
		s.upgrader.CheckOrigin = s.oc()
		c, err := s.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade to websocket conn:", err)
			return
		}
		pmt, pm, err := c.ReadMessage()
		if err != nil {
			log.Printf("Error reading init message: %v\n", err)
			return
		}
		if pmt == websocket.BinaryMessage {
			prs := struct {
				ID string `json:"id"`
			}{}
			decryptedInitMsg, err := s.enc.passwordDecrypt(pm, s.serverKey)
			if err != nil {
				log.Printf("Error decrypting init message: %v\n", err)
			}
			if err := json.Unmarshal(decryptedInitMsg, &prs); err != nil {
				log.Printf("Error parsing init message: %v\n", err)
			}
			if len(prs.ID) != 64 {
				log.Printf("Error ID length != 64: %v\n", prs.ID)
				return
			}
			if !slices.Contains(s.users, prs.ID) {
				log.Printf("User %s not found in USERS\n", prs.ID)
				return
			}
			currentUserID = prs.ID
			s.mut.Lock()
			s.websockets[prs.ID] = c
			s.mut.Unlock()
			log.Printf("User  %s Connected from %s\n", prs.ID, r.RemoteAddr)
		} else {
			log.Printf("Error parsing init message: %v\n", string(pm))
			return
		}
		// set ping handler
		c.SetPingHandler(func(m string) error {
			if err := c.WriteMessage(websocket.PongMessage, []byte("pong")); err != nil {
				return errors.New("websocket pong: " + err.Error())
			}
			return nil
		})
		s.mut.Lock()
		mq, ok := s.messageQueue[currentUserID]
		s.mut.Unlock()
		if ok {
			s.mut.Lock()
			for i := 0; i < len(mq); i++ {
				if err := s.websockets[currentUserID].WriteMessage(websocket.TextMessage, mq[i]); err != nil {
					log.Printf("Error writing message: %v\n", err)
					break
				}
			}
			delete(s.messageQueue, currentUserID)
			s.mut.Unlock()
		}
		for {
			messageType, message, err := c.ReadMessage()
			if err != nil {
				log.Printf("removing websocket session for %s\n", currentUserID)
				s.removeConnection(currentUserID)
				return
			}
			switch messageType {
			case websocket.TextMessage:
				decryptedMsg, err := s.enc.passwordDecrypt(message, s.serverKey)
				if err != nil {
					log.Printf("Error decrypting message: %v\n", err)
					return
				}
				mt := &MessageTemplate{}
				if err := json.Unmarshal(decryptedMsg, mt); err != nil {
					log.Printf("Error parsing message: %v\n", err)
					return
				}
				s.mut.Lock()
				wc, ok := s.websockets[mt.ID]
				s.mut.Unlock()
				if ok {
					s.mut.Lock()
					if err := wc.WriteMessage(messageType, message); err != nil {
						log.Printf("Error writing message: %v\n", err)
					}
					s.mut.Unlock()
					break
				} else {
					s.mut.Lock()
					s.messageQueue[mt.ID] = append(s.messageQueue[mt.ID], message)
					s.mut.Unlock()
				}
			default:
				fmt.Printf("Unused message type: %v\n", messageType)
				continue
			}
		}
	})
	if err := http.ListenAndServeTLS(fmt.Sprintf(":%d", s.tlsPort), s.cert, s.key, nil); err != nil {
		log.Printf("ListenAndServeTLS: %v\n", err)
	}
}

func main() {
	c := &Config{}
	if err := json.Unmarshal(configFile, c); err != nil {
		log.Fatalf("Error parsing config file: %v\n", err)
	}
	s := &Server{
		enc: &Encryption{
			iter: 100000,
		},
		serverKey: c.ServerKey,
		endpoint:  c.Endpoint,
		tlsPort:   c.Port,
		cert:      c.CertFile,
		key:       c.KeyFile,
		users:     c.Users,
	}
	s.websockets = make(map[string]*websocket.Conn)
	s.messageQueue = make(map[string][][]byte)
	log.Printf("Starting server on port %d\n", c.Port)
	s.start()
}
