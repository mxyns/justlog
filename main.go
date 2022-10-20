package main

import (
	"embed"
	"flag"

	"github.com/mxyns/justlog/api"
	"github.com/mxyns/justlog/archiver"
	"github.com/mxyns/justlog/bot"
	"github.com/mxyns/justlog/config"
	"github.com/mxyns/justlog/filelog"
	"github.com/mxyns/justlog/helix"
)

// content holds our static web server content.
//go:embed web/build/*
var assets embed.FS

func main() {

	configFile := flag.String("config", "config.json", "json config file")
	flag.Parse()

	cfg := config.NewConfig(*configFile)

	fileLogger := filelog.NewFileLogger(cfg.LogsDirectory)
	helixClient := helix.NewClient(cfg.ClientID, cfg.ClientSecret)
	go helixClient.StartRefreshTokenRoutine()

	if cfg.Archive {
		archiver := archiver.NewArchiver(cfg.LogsDirectory)
		go archiver.Boot()
	}

	bot := bot.NewBot(cfg, &helixClient, &fileLogger)

	apiServer := api.NewServer(cfg, bot, &fileLogger, &helixClient, assets)
	go apiServer.Init()

	bot.Connect()
}
