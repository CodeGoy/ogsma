package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

type ClientConfig struct {
	Addr      string `json:"addr"`
	KeyStore  string `json:"keystore"`
	Endpoint  string `json:"endpoint"`
	ServerKey string `json:"serverkey"`
}

type ServerConfig struct {
	Port      int      `json:"port"`
	Endpoint  string   `json:"endpoint"`
	CertFile  string   `json:"certFile"`
	KeyFile   string   `json:"keyFile"`
	Users     []string `json:"users"`
	ServerKey string   `json:"serverkey"`
}

func main() {
	var ep, ks, addr, cert, key, tp, opf, ukfs, sk string
	var port int
	flag.StringVar(&sk, "sk", "", "server encryption key")
	flag.StringVar(&ukfs, "ukfs", "", "comma-separated list of user keystore files")
	flag.StringVar(&opf, "opf", "config.json", "output file for client config")
	flag.StringVar(&tp, "type", "", "type of config (client, server)")
	flag.StringVar(&key, "key", "", "TLS private key")
	flag.StringVar(&cert, "cert", "", "TLS cert file")
	flag.StringVar(&ep, "endpoint", "ws", "websocket endpoint")
	flag.StringVar(&ks, "keystore", "", "encrypted keystore string")
	flag.StringVar(&addr, "addr", "10.1.10.194", "address of server (10.1.10.194)")
	flag.IntVar(&port, "port", 0, "server port")
	flag.Parse()
	// TODO : nil checking of relevant vars

	if port == 0 {
		log.Fatal("port number required")
		return
	}
	switch tp {
	case "client":
		if cjb, err := json.Marshal(&ClientConfig{
			Addr:      fmt.Sprintf("%s:%d", addr, port),
			KeyStore:  ks,
			Endpoint:  ep,
			ServerKey: sk,
		}); err != nil {
			log.Fatalf("Error marshalling config: %v\n", err)
		} else {
			if err := os.WriteFile(opf, cjb, 0666); err != nil {
				log.Fatalf("Error writing config.json: %v\n", err)
			}
		}
	case "server":
		var clientIdList []string
		for _, s := range strings.Split(ukfs, ",") {
			keystoreFileBytes, err := os.ReadFile(fmt.Sprintf("%s.keyshare", s))
			if err != nil {
				log.Fatalf("Error opening keystore file %s: %v\n", s, err)
			}
			keystoreID := &struct {
				ID string `json:"id"`
			}{}
			if err := json.Unmarshal(keystoreFileBytes, keystoreID); err != nil {
				log.Fatalf("Error unmarshalling keystore file %s: %v\n", s, err)
			}
			clientIdList = append(clientIdList, keystoreID.ID)
		}
		if sjb, err := json.Marshal(&ServerConfig{
			Port:      port,
			Endpoint:  ep,
			CertFile:  cert,
			KeyFile:   key,
			Users:     clientIdList,
			ServerKey: sk,
		}); err != nil {
			log.Fatalf("Error marshalling config: %v\n", err)
		} else {
			if err := os.WriteFile("server_config.json", sjb, 0666); err != nil {
				log.Fatalf("Error writing server_config.json: %v\n", err)
			}
		}
	default:
		log.Fatalf("Unsupported type of config (client, server)")
	}
}
