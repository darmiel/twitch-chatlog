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
	"sync"
	"time"
)

///

func main() {
	defHandler := clilog.Default
	log.SetHandler(defHandler)

	// TODO: Debug
	log.SetLevel(log.DebugLevel)
	var mu sync.Mutex

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

	// load channels to listen messages
	var channels []*ListeningChannel
	if tx := db.Where("active = true").Find(&channels); tx.Error != nil {
		log.WithError(tx.Error).Error("retrieving listening channels failed")
		return
	}
	if len(channels) == 0 {
		log.Error("no channels to listen")
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
				for i, c := range channels {
					if (i+1)%10 == 0 {
						time.Sleep(11 * time.Second)
					}
					if err = client.Write("JOIN #" + c.ChannelName); err != nil {
						log.WithError(err).
							WithField("channel", c.ChannelName).
							Error("error joining channel")
					} else {
						log.WithField("channel", c.ChannelName).
							Info("joined channel")
					}
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
				log.WithField("rows affected", tx.RowsAffected).
					Debugf("💾 %s", msg.String())
			}
		}),
	}
	client := irc.NewClient(conn, ic)

	go func() {
		for {
			time.Sleep(5 * time.Second)
			log.Debug("checking for new channels to listen")

			var next []*ListeningChannel
			if tx := db.Where("active = true").Find(&next); tx.Error != nil {
				log.WithError(tx.Error).Error("retrieving listening channels failed")
				return
			}
			join, leave := CompareArrays(channels, next)

			mu.Lock()
			channels = next
			mu.Unlock()

			// TODO: remove duplicate
			for i, c := range join {
				if (i+1)%10 == 0 {
					log.Debug("waiting 11 seconds")
					time.Sleep(11 * time.Second)
				}
				if err := client.Write("JOIN #" + c.ChannelName); err != nil {
					log.WithError(err).
						WithField("channel", c.ChannelName).
						Error("error joining (guard)")
				} else {
					log.WithField("channel", c.ChannelName).
						Info("joined channel (guard)")
				}
			}

			for i, c := range leave {
				if (i+1)%10 == 0 {
					log.Debug("waiting 11 seconds")
					time.Sleep(11 * time.Second)
				}
				if err := client.Write("PART #" + c.ChannelName); err != nil {
					log.WithError(err).
						WithField("channel", c.ChannelName).
						Error("error leaving channel (guard)")
				} else {
					log.WithField("channel", c.ChannelName).
						Info("left channel (guard)")
				}
			}
		}
	}()

	if err = client.Run(); err != nil {
		panic(err)
	}
}
