package main

import (
	"context"
	"flag"
	"github.com/apex/log"
	clilog "github.com/apex/log/handlers/cli"
	"github.com/davecgh/go-spew/spew"
	toml2 "github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"gopkg.in/irc.v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// flags
var (
	verboseFlag = flag.Bool("verbose", false, "Enable verbose logging")
)

func init() {
	log.SetLevel(log.InfoLevel)

	flag.Parse()
	if *verboseFlag {
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	log.SetHandler(clilog.Default)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// load configuration
	k := koanf.New(".")

	// load defaults
	if err := k.Load(structs.Provider(Config{
		TwitchNick: "justinfan30316",
		TwitchUser: "justinfan30316",
		TwitchName: "justinfan30316",
		TwitchPass: "justinfan30316",
	}, "koanf"), nil); err != nil {
		log.WithError(err).Error("error loading defaults")
		return
	}

	// load config from TOML file
	if err := k.Load(file.Provider("settings.toml"), toml2.Parser()); err != nil {
		log.WithError(err).Warn("error loading config")
		// we can continue without a config file
	}

	// load config from env
	if err := k.Load(env.Provider("TC_", ".", func(s string) string {
		return strings.Replace(
			strings.ToLower(strings.TrimPrefix(s, "TC_")),
			"__", ".", -1,
		)
	}), nil); err != nil {
		log.WithError(err).Warn("error loading env")
		// we can continue without env
	}

	var config Config
	if err := k.Unmarshal("", &config); err != nil {
		log.WithError(err).Error("error unmarshalling config")
		return
	}

	spew.Dump(config)

	log.Info("Connecting to Database ...")
	db, err := gorm.Open(postgres.Open(config.PostgresDSN))
	if err != nil {
		log.WithError(err).Error("connecting to postgres failed")
		return
	}
	log.Info("Connected to Postgres! Migrating models ...")

	// migrate models
	if err = db.AutoMigrate(
		&User{},
		&Channel{},
		&Message{},
		&ListeningChannel{},
	); err != nil {
		log.WithError(err).Error("error migrating")
		return
	}

	h := &handler{
		Context: ctx,
		db:      db,
	}

	// web server
	if config.WebBind != "" {
		log.Infof("Spinning up web server at %s/ping", config.WebBind)

		http.HandleFunc("/ping", h.handlePing)
		go func() {
			if err := http.ListenAndServe(config.WebBind, nil); err != nil {
				log.WithError(err).Error("failed listening and serving")
				os.Exit(1)
				return
			}
		}()
	}

	ic := irc.ClientConfig{
		Nick:    config.TwitchNick,
		User:    config.TwitchUser,
		Name:    config.TwitchName,
		Pass:    config.TwitchPass,
		Handler: irc.HandlerFunc(h.handleIRCMessage),
	}

	log.Info("Connecting to IRC ...")
	conn, err := net.Dial("tcp", "irc.chat.twitch.tv:6667")
	if err != nil {
		log.WithError(err).Errorf("connection to IRC failed.")
		return
	}

	log.Info("Connected. Building IRC Client ...")
	client := irc.NewClient(conn, ic)

	// start checking for new channels every 5 seconds
	go h.startCheckAndJoinTicker(client)

	log.Info("Running client ...")
	if err = client.RunContext(ctx); err != nil {
		log.WithError(err).Error("error running client")
	}
}

func joinChannel(client *irc.Client, channel *ListeningChannel) {
	if err := client.Write("JOIN #" + channel.ChannelName); err != nil {
		log.WithError(err).
			WithField("channel", channel.ChannelName).
			Error("error joining channel")
	} else {
		log.WithField("channel", channel.ChannelName).
			Info("joined channel")
	}
}

func leaveChannel(client *irc.Client, channel *ListeningChannel) {
	if err := client.Write("PART #" + channel.ChannelName); err != nil {
		log.WithError(err).
			WithField("channel", channel.ChannelName).
			Error("error leaving channel")
	} else {
		log.WithField("channel", channel.ChannelName).
			Info("left channel")
	}
}
