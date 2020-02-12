package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	net "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

// Transaction is Data struct for a single transaction
type Transaction struct {
	Requester string
	Activity  string
	Device    string
}

// Policy is difined by owner
type Policy struct {
	Requester, Activity, Device string
	Permission                  bool
}

type dataPasToGenerate struct {
	Transaction Transaction
	Policy      Policy
}

// Block content
type Block struct {
	Index        int
	Timestamp    string
	Transactions []Transaction
	Policies     []Policy
	RootHashT    string
	RootHashP    string
	Hash         string
	PrevHash     string
	Nonce        string
}

// Message is a format for received information
type Message struct {
	Transactions []Transaction
	Policies     []Policy
}

// Blockchain is an array of blocks
var Blockchain []Block

const difficulty = 5

var mutex = &sync.Mutex{}

func main() {
	t := time.Now()
	genesisBlock := Block{}
	genesisBlock = Block{Index: 0, Timestamp: t.String(), Transactions: []Transaction{},
		Policies: []Policy{}, RootHashT: "", RootHashP: "", Hash: calculateHash(genesisBlock), PrevHash: "", Nonce: ""}
	Blockchain = append(Blockchain, genesisBlock)

	listenF := flag.Int("l", 0, "wait for incoming connections")
	target := flag.String("d", "", "Target peer to dial")
	secio := flag.Bool("secio", false, "enable secio")
	seed := flag.Int64("seed", 0, "set random seed for id generation")
	flag.Parse()

	if *listenF == 0 {
		log.Fatal("Please provide a port with -l flag")
	}

	ha, err := makeBasicHost(*listenF, *secio, *seed)
	if err != nil {
		log.Fatal(err)
	}

	if *target == "" {
		log.Println("listening for connections")
		ha.SetStreamHandler("/p2p/1.0.0", handleStream)
		select {}
	} else {
		ha.SetStreamHandler("/p2p/1.0.0", handleStream)
		ipfsaddr, err := ma.NewMultiaddr(*target)
		if err != nil {
			log.Fatalln(err)
		}
		pid, err := ipfsaddr.ValueForProtocol(ma.P_IPFS)
		if err != nil {
			log.Fatalln(err)
		}

		peerid, err := peer.IDB58Decode(pid)
		if err != nil {
			log.Fatalln(err)
		}
		targetPeerAddr, _ := ma.NewMultiaddr(
			fmt.Sprintf("/ipfs/%s", peer.IDB58Encode(peerid)))
		targetAddr := ipfsaddr.Decapsulate(targetPeerAddr)
		ha.Peerstore().AddAddr(peerid, targetAddr, pstore.PermanentAddrTTL)

		log.Println("opening stream")
		s, err := ha.NewStream(context.Background(), peerid, "/p2p/1.0.0")
		if err != nil {
			log.Fatalln(err)
		}
		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
		go writeData(rw)
		go readData(rw)

		select {}
	}
}

func isBlockValid(newBlock, oldBlock Block) bool {
	if oldBlock.Index+1 != newBlock.Index {
		return false
	}

	if oldBlock.Hash != newBlock.Hash {
		return false
	}

	if calculateHash(newBlock) != newBlock.Hash {
		return false
	}

	return true
}

func calculateHash(block Block) string {
	record := strconv.Itoa(block.Index) + block.Timestamp + block.RootHashT + block.RootHashP + block.PrevHash + block.Nonce
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

func generateBlock(oldBlock Block, data Message) Block {
	var newBlock Block

	t := time.Now()

	newBlock.Index = oldBlock.Index + 1
	newBlock.Timestamp = t.String()
	newBlock.Transactions = data.Transactions
	newBlock.Policies = data.Policies
	newBlock.RootHashT = calculateRootHashT(data.Transactions)
	newBlock.RootHashP = calculateRootHashP(data.Policies)
	newBlock.PrevHash = oldBlock.Hash
	newBlock.Hash = calculateHash(newBlock)

	return newBlock
}

func calculateRootHashT(Transactions []Transaction) string {
	if len(Transactions) == 0 {
		return ""
	}
	var hash []string
	var rawData string
	h := sha256.New()
	for len(hash) != 1 {
		if len(hash) == 0 {
			for i := 0; i < len(Transactions); i++ {
				h.Reset()
				rawData = Transactions[i].Requester + Transactions[i].Activity + Transactions[i].Device
				h.Write([]byte(rawData))
				hashed := hex.EncodeToString(h.Sum(nil))
				hash = append(hash, hashed)
			}
		} else {
			var newHash []string
			for i := 0; i < len(hash); i = i + 2 {
				if i == len(hash)-1 {
					rawData = hash[i]
				} else {
					rawData = hash[i] + hash[i+1]
				}
				h.Reset()
				h.Write([]byte(rawData))
				newHash = append(newHash, hex.EncodeToString(h.Sum((nil))))
			}
			hash = newHash
		}
	}
	return hash[0]
}

func calculateRootHashP(Policies []Policy) string {
	if len(Policies) == 0 {
		return ""
	}
	var hash []string
	var rawData string
	h := sha256.New()
	for len(hash) != 1 {
		if len(hash) == 0 {
			for i := 0; i < len(Policies); i++ {
				h.Reset()
				rawData = Policies[i].Requester + Policies[i].Activity + Policies[i].Device + strconv.FormatBool(Policies[i].Permission)
				h.Write([]byte(rawData))
				hashed := hex.EncodeToString(h.Sum(nil))
				hash = append(hash, hashed)
			}
		} else {
			var newHash []string
			for i := 0; i < len(hash); i = i + 2 {
				if i == len(hash)-1 {
					rawData = hash[i]
				} else {
					rawData = hash[i] + hash[i+1]
				}
				h.Reset()
				h.Write([]byte(rawData))
				newHash = append(newHash, hex.EncodeToString(h.Sum((nil))))
			}
			hash = newHash
		}
	}
	return hash[0]
}

func makeBasicHost(listenPort int, secio bool, randSeed int64) (host.Host, error) {
	var r io.Reader
	if randSeed == 0 {
		r = rand.Reader
	} else {
		r = mrand.New(mrand.NewSource(randSeed))
	}

	// Generate private and public key
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", listenPort)),
		libp2p.Identity(priv),
	}

	basicHost, err := libp2p.New(context.Background(), opts...)
	if err != nil {
		return nil, err
	}

	hostAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", basicHost.ID().Pretty()))

	addrs := basicHost.Addrs()
	var addr ma.Multiaddr
	for _, i := range addrs {
		if strings.HasPrefix(i.String(), "/ip4") {
			addr = i
			break
		}
	}
	fullAddr := addr.Encapsulate(hostAddr)
	log.Printf("Address of this node is: %s", fullAddr)

	return basicHost, nil
}

func handleStream(s net.Stream) {
	log.Println("Got a new stream!")

	//Create a buffer stream for non blocking read and write.
	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	go readData(rw)
	go writeData(rw)
}

func readData(rw *bufio.ReadWriter) {

	for {
		str, err := rw.ReadString('\n')
		if err != nil {
			log.Println(err)
		}

		if str == "" {
			return
		}
		if str != "\n" {

			chain := make([]Block, 0)
			if err := json.Unmarshal([]byte(str), &chain); err != nil {
				log.Fatal(err)
			}

			mutex.Lock()
			if len(chain) > len(Blockchain) {
				Blockchain = chain
				bytes, err := json.MarshalIndent(Blockchain, "", "  ")
				if err != nil {

					log.Fatal(err)
				}
				// Green console color: 	\x1b[32m
				// Reset console color: 	\x1b[0m
				fmt.Printf("\x1b[32m%s\x1b[0m> ", string(bytes))
			}
			mutex.Unlock()
		}
	}
}

func writeData(rw *bufio.ReadWriter) {

	go func() {
		for {
			time.Sleep(5 * time.Second)
			mutex.Lock()
			bytes, err := json.Marshal(Blockchain)
			if err != nil {
				log.Println(err)
			}
			mutex.Unlock()

			mutex.Lock()
			rw.WriteString(fmt.Sprintf("%s\n", string(bytes)))
			rw.Flush()
			mutex.Unlock()

		}
	}()

	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("Menu Pilihan")
		fmt.Println("1. Tambah Transaksi")
		fmt.Println("2. Lihat Blockchain")
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		sendData = strings.Replace(sendData, "\n", "", -1)
		if sendData == "1" {
			transaksi := []Transaction{}
			transaksi = append(transaksi, Transaction{Activity: "Get Data", Device: "lkj3k5jhjkhkjhd", Requester: "kjwekjk3j5k43jk"})
			block := generateBlock(Blockchain[len(Blockchain)-1], Message{Transactions: transaksi})
			Blockchain = append(Blockchain, block)
		} else if sendData == "2" {
			fmt.Println(Blockchain)
		}
	}
}
