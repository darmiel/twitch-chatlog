package main

import (
	"fmt"
	"github.com/apex/log"
	clilog "github.com/apex/log/handlers/cli"
	"gopkg.in/irc.v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net"
)

const (
	anonymousUser = "justinfan30316"
)

// Database
const (
	PostgresHost = "localhost"
	PostgresUser = "postgres"
	PostgresPass = "123456"
	PostgresDb   = "postgres"
	PostgresPort = "45433"
	PostgresTZ   = "Europe/Berlin"
)

func softPanic(err error) {
	if err != nil {
		panic(err)
	}
}

///

func main() {
	// Apex Logger CLI Handler
	log.SetHandler(clilog.Default)

	defHandler := clilog.Default
	log.SetHandler(log.HandlerFunc(func(entry *log.Entry) error {
		// catch errors
		if entry.Level == log.ErrorLevel {
			fmt.Println("[LOG] Catched error:", entry.Message)
		}
		return defHandler.HandleLog(entry)
	}))

	// Database
	log.Info("Connecting to Database ...")
	db, err := gorm.Open(postgres.Open(fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=%s",
		PostgresHost, PostgresUser, PostgresPass, PostgresDb, PostgresPort, PostgresTZ,
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
	config := irc.ClientConfig{
		Nick: anonymousUser,
		User: anonymousUser,
		Name: anonymousUser,
		Handler: irc.HandlerFunc(func(client *irc.Client, message *irc.Message) {
			if message.Command == "001" {
				// use the Tags feature
				// https://dev.twitch.tv/docs/irc/guide/#twitch-irc-capabilities
				softPanic(client.Write("CAP REQ :twitch.tv/tags"))

				// join channels
				// TODO: Join channels
				softPanic(client.Write("JOIN #unsympathisch_tv"))
			} else if message.Command == "PRIVMSG" {
				// meta data
				var msg *Message
				if msg, err = ParseIRCMessage(message); err != nil {
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
	client := irc.NewClient(conn, config)
	if err = client.Run(); err != nil {
		panic(err)
	}
}
