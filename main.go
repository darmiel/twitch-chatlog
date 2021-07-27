package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/apex/log"
	clilog "github.com/apex/log/handlers/cli"
	"github.com/getsentry/sentry-go"
	"gopkg.in/irc.v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net"
	"time"
)

///

func main() {
	defHandler := clilog.Default
	log.SetHandler(defHandler)

	// load config
	var config *Config
	if _, err := toml.DecodeFile("settings.toml", &config); err != nil {
		log.WithError(err).Error("error loading config")
		return
	}
	if config == nil {
		log.Error("config was null")
		return
	}

	// load sentry
	if config.SentryDSN != "" {
		log.Info("Using Sentry")
		if err := sentry.Init(sentry.ClientOptions{
			Dsn: config.SentryDSN,
		}); err != nil {
			log.WithError(err).Error("error initializing sentry")
			return
		}
		defer sentry.Flush(2 * time.Second)

		// catch errors and send 'em to sentry
		log.SetHandler(log.HandlerFunc(func(entry *log.Entry) error {
			if entry.Level == log.ErrorLevel {
				sentry.CaptureException(fmt.Errorf(entry.Message))
			}
			return defHandler.HandleLog(entry)
		}))
	}

	// Database
	log.Info("Connecting to Database ...")
	db, err := gorm.Open(postgres.Open(fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=%s",
		config.PostgresHost, config.PostgresUser, config.PostgresPass, config.PostgresDb, config.PostgresPort, config.PostgresTZ,
	)))

	if err != nil {
		log.WithError(err).Error("connecting to postgres failed")
		return
	}
	log.Info("Connected to postgres. Migrating ...")

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

	log.Info("Connecting to IRC ...")
	conn, err := net.Dial("tcp", "irc.chat.twitch.tv:6667")
	if err != nil {
		log.WithError(err).Error("connection to IRC failed")
		return
	}

	log.Info("Connected. Building IRC Client ...")
	ic := irc.ClientConfig{
		Nick: config.TwitchNick,
		User: config.TwitchUser,
		Name: config.TwitchName,
		Pass: config.TwitchPass,
		Handler: irc.HandlerFunc(func(client *irc.Client, message *irc.Message) {
			if message.Command == "001" {
				// use the Tags feature
				// https://dev.twitch.tv/docs/irc/guide/#twitch-irc-capabilities
				if err = client.Write("CAP REQ :twitch.tv/tags"); err != nil {
					log.WithError(err).Error("error requesting tags")
				}

				// join channels
				// TODO: Join channels
				if err = client.Write("JOIN #unsympathisch_tv"); err != nil {
					log.WithError(err).Error("error joining channel")
				}
			} else if message.Command == "PRIVMSG" {
				// meta data
				var msg *Message
				if msg, err = ParseIRCMessage(message); err != nil {
					log.WithError(err).Error("error parsing message")
					return
				}
				tx := db.Create(msg)
				if tx.Error != nil {
					log.WithError(tx.Error).Error("Failed to save chat message")
					return
				}
				log.Infof("[Chat] Saved message: %+v (%v rows affected)", msg, tx.RowsAffected)
			}
		}),
	}
	client := irc.NewClient(conn, ic)
	if err = client.Run(); err != nil {
		panic(err)
	}
}
