package main

import (
	"fmt"
	"os"
	"time"

	"github.com/everFinance/goether"
	"github.com/hymatrix/hymx/sdk"
	registrySchema "github.com/hymatrix/hymx/vmm/core/registry/schema"
	"github.com/hymatrix/hymx/vmm/core/token/schema"
	"github.com/permadao/goar"
)

var (
	url = GetEnvWith("VMDOCKER_URL", "http://127.0.0.1:8080")

	s *sdk.SDK

	module    = GetEnvWith("VMDOCKER_MODULE_ID", "4sX9Uo5-Qk37yUOMLCMrwnm4S3Wfu3Fp7QCSRN0oeoU")
	scheduler = GetEnvWith("VMDOCKER_SCHEDULER", "0x972AeD684D6f817e1b58AF70933dF1b4a75bfA51")

	mainNode = registrySchema.Node{
		Name: "test",
		Desc: "test node",
		URL:  url,
	}
)

func initExampleSDK() {
	if s != nil {
		return
	}

	prvKey := GetEnv("VMDOCKER_PRIVATE_KEY")
	signer, err := goether.NewSigner(prvKey)
	if err != nil {
		panic(err)
	}
	bundler, err := goar.NewBundler(signer)
	if err != nil {
		panic(err)
	}
	s = sdk.NewFromBundler(url, bundler)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("please input cmd, ex: pingpong, sendMessage, spawn, eval, eval2, receive, receive2, reply, inbox, result, checkpoint, ollama, recover1, recover2, openclaw_spawn, openclaw_chat, openclaw_tg, openclaw_pair, claude_spawn, claude_chat")
		os.Exit(1)
	}

	initExampleSDK()

	cmd := os.Args[1]
	switch cmd {
	case "init":
		tokenPid, err := initToken()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		initRegistry(tokenPid, mainNode)
	case "transfer":
		if len(os.Args) < 3 {
			fmt.Println("please provide to address for transfer")
			os.Exit(1)
		}
		toAddr := os.Args[2]
		err := transfer(s, toAddr, schema.StakeMinAmount)
		if err != nil {
			fmt.Printf("transfer err: %v\n", err)
		} else {
			fmt.Println("transfer success to ", toAddr)
		}
	case "pingpong":
		pingpong()
	case "module":
		genModule()
	case "spawn":
		spawn()
	case "spawnChild":
		spawnChild()
	case "eval":
		eval()
	case "receive":
		receive()
	case "receive2":
		receive2()
	case "reply":
		reply()
	case "inbox":
		inbox()
	case "ollama":
		ollama()
	case "stress":
		doTansfer()
	case "test":
		err := agentTestRT()
		if err != nil {
			fmt.Printf("agentTestRT err: %v\n", err)
		} else {
			fmt.Println("agentTestRT success")
		}
	case "openclaw_spawn":
		spawnOpenclaw()
	case "openclaw_chat":
		id := spawnOpenclaw()
		if id != "" {
			time.Sleep(1 * time.Second)
			chatOpenclaw(id)
		}
	case "openclaw_tg":
		id := spawnOpenclaw()
		if id != "" {
			time.Sleep(1 * time.Second)
			telegramOpenclaw(id)
		}
	case "openclaw_pair":
		if len(os.Args) < 4 {
			fmt.Println("usage: openclaw_pair <pid> <pair_code>")
			os.Exit(1)
		}
		id := os.Args[2]
		code := os.Args[3]
		pairTgOpenclaw(id, code)
	case "claude_spawn":
		spawnClaude()
	case "claude_chat":
		target, command, shouldSpawn, err := resolveClaudeChatArgs(os.Args[2:])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if shouldSpawn {
			target = spawnClaude()
			if target != "" {
				time.Sleep(1 * time.Second)
			}
		}
		if target != "" {
			chatClaude(target, command)
		}
	default:
		fmt.Printf("unknown cmd: %s\n", cmd)
		os.Exit(1)
	}
}
