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
	"gorm.io/gorm/clause"
	"net"
	"sync"
	"time"
)

var (
	currentChannels   []*ListeningChannel
	currentChannelsMu sync.Mutex
	checkingMu        sync.Mutex
)

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

func checkAndJoinLeave(db *gorm.DB, client *irc.Client) {
	currentChannelsMu.Lock()
	defer currentChannelsMu.Unlock()

	var nextChannels []*ListeningChannel
	if tx := db.Where("active = true").Find(&nextChannels); tx.Error != nil {
		log.WithError(tx.Error).Error("retrieving listening channels failed")
		return
	}
	if len(nextChannels) == 0 {
		log.Warn("Not joining any rooms")
	}

	join, leave := CompareArrays(currentChannels, nextChannels)
	delay := 550 * time.Millisecond
	if len(join)+len(leave) == 0 {
		return
	} else if len(join)+len(leave) <= 10 {
		delay = 0
	}
	log.Debugf("Join/Leave delay set to %v to prevent getting flood rated", delay)

	for _, j := range join {
		joinChannel(client, j)
		time.Sleep(delay)
	}
	for _, l := range leave {
		leaveChannel(client, l)
		time.Sleep(delay)
	}

	currentChannels = nextChannels
}

func main() {
	log.SetHandler(clilog.Default)
	log.SetLevel(log.DebugLevel)

	/// Config
	var config *Config
	if _, err := toml.DecodeFile("settings.toml", &config); err != nil {
		log.WithError(err).Error("error loading config")
		return
	}
	if config == nil {
		log.Error("config was null")
		return
	}

	/// Sentry
	if config.SentryDSN != "" {
		log.Info("Using Sentry for error monitoring :)")
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
			return clilog.Default.HandleLog(entry)
		}))
	}

	/// Database
	log.Info("Connecting to Database ...")
	db, err := gorm.Open(postgres.Open(fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=%s",
		config.PostgresHost, config.PostgresUser, config.PostgresPass, config.PostgresDb,
		config.PostgresPort, config.PostgresTZ,
	)))
	if err != nil {
		log.WithError(err).Error("connecting to postgres failed")
		return
	}
	log.Info("Connected to postgres. Migrating models ...")

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
				if err = client.Write("CAP REQ :twitch.tv/tags twitch.tv/commands"); err != nil {
					log.WithError(err).Error("error requesting tags and commands")
				}

				// Join new channels
				checkingMu.Lock()
				checkAndJoinLeave(db, client)
				checkingMu.Unlock()
			} else if message.Command == "PRIVMSG" {
				/// ===========================
				/// PRIVMSG
				/// Send a message to a channel
				/// ===========================
				var msg *Message
				if msg, err = ParseIRCMessage(message); err != nil {
					log.WithError(err).Error("error parsing message")
					return
				}
				tx := db.Clauses(clause.OnConflict{DoNothing: true}).Create(msg)
				if tx.Error != nil {
					log.WithError(tx.Error).Error("Failed to save chat message")
					return
				}
				log.WithField("rows affected", tx.RowsAffected).
					Debugf("💾 %s", msg.String())
			} else if message.Command == "CLEARMSG" {
				/// ============================================
				/// CLEARMSG
				/// Single message removal on a channel.
				/// This is triggered by /delete <target-msg-id>
				/// ============================================
				target, ok := message.GetTag("target-msg-id")
				if !ok {
					log.Warn("Message deleted but no message reference found")
					return
				}

				t := time.Now()
				tx := db.Updates(&Message{
					ID:      target,
					Deleted: &t,
				})
				if tx.Error != nil {
					log.WithError(tx.Error).Error("error updating message (deleted)")
					return
				}
				log.WithField("rows affected", tx.RowsAffected).
					WithField("target message", target).
					Debug("Updated message (delete)")
			}
		}),
	}
	client := irc.NewClient(conn, ic)

	go func() {
		for {
			time.Sleep(10 * time.Second)
			checkingMu.Lock()
			log.Debug("Checking for new channels to listen")
			checkAndJoinLeave(db, client)
			checkingMu.Unlock()
		}
	}()

	if err = client.Run(); err != nil {
		log.WithError(err).Error("error running client")
	}
}
