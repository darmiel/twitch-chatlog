package main

import (
	"context"
	"fmt"
	"github.com/apex/log"
	"gopkg.in/irc.v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/http"
	"sync"
	"time"
)

type handler struct {
	context.Context

	db                  *gorm.DB
	lastIncomingMessage time.Time
	lastDeletedMessage  time.Time

	currentChannels   []*ListeningChannel
	currentChannelsMu sync.Mutex
	checkingMu        sync.Mutex
}

func (h *handler) handleIRCMessage(client *irc.Client, message *irc.Message) {
	// Init Client
	if message.Command == "001" {
		log.Debug("Connected to IRC and got 001")

		// use the Tags feature
		// https://dev.twitch.tv/docs/irc/guide/#twitch-irc-capabilities
		if err := client.Write("CAP REQ :twitch.tv/tags twitch.tv/commands"); err != nil {
			log.WithError(err).Error("error requesting tags and commands")
		}

		// Join new channels
		h.checkingMu.Lock()
		h.checkAndJoinLeave(client)
		h.checkingMu.Unlock()
		return
	}

	// ===========================
	// PRIVMSG
	// Send a message to a channel
	// ===========================
	if message.Command == "PRIVMSG" {
		msg, err := ParseIRCMessage(message)
		if err != nil {
			log.WithError(err).Error("error parsing message")
			return
		}
		tx := h.db.Clauses(clause.OnConflict{DoNothing: true}).Create(msg)
		if tx.Error != nil {
			log.WithError(tx.Error).Error("Failed to save chat message")
			return
		}
		log.WithField("rows affected", tx.RowsAffected).
			WithField("user", message.Name).
			WithField("channel", message.Param(0)).
			Debugf("💾 %s", msg.String())
		h.lastIncomingMessage = time.Now()
		return
	}

	// ============================================
	// CLEARMSG
	// Single message removal on a channel.
	// This is triggered by /delete <target-msg-id>
	// ============================================
	if message.Command == "CLEARMSG" {
		log.WithField("channel", message.Param(0)).
			WithField("user", message.Name).
			Debug("Received message deletion")

		target, ok := message.GetTag("target-msg-id")
		if !ok {
			log.Warn("Message deleted but no message reference found")
			return
		}

		t := time.Now()
		tx := h.db.Updates(&Message{
			ID:      target,
			Deleted: &t,
		})
		if tx.Error != nil {
			log.WithError(tx.Error).Error("error updating message (deleted)")
			return
		}
		log.WithField("rows affected", tx.RowsAffected).
			WithField("target message", target).
			Debug("❌ Updated message (delete)")
		h.lastDeletedMessage = time.Now()
	}
}

func (h *handler) checkAndJoinLeave(client *irc.Client) {
	h.currentChannelsMu.Lock()
	defer h.currentChannelsMu.Unlock()

	var nextChannels []*ListeningChannel
	if tx := h.db.Where("active = true").Find(&nextChannels); tx.Error != nil {
		log.WithError(tx.Error).Error("retrieving listening channels failed")
		return
	}
	if len(nextChannels) == 0 {
		log.Warn("Not joining any rooms")
	}

	join, leave := CompareArrays(h.currentChannels, nextChannels)

	// if we don't have any join/leave, we don't need to do anything
	if len(join)+len(leave) == 0 {
		return
	}

	delay := 550 * time.Millisecond
	if len(join)+len(leave) <= 10 {
		delay = 0
	}
	log.Debugf("Join/Leave delay set to %v to prevent getting flood rated", delay)

	// join channels
	for _, j := range join {
		select {
		case <-h.Context.Done():
			log.Info("Context canceled, stopping join operations")
			return
		default:
			joinChannel(client, j)
			time.Sleep(delay)
		}
	}

	// leave channels
	for _, l := range leave {
		select {
		case <-h.Context.Done():
			log.Info("Context canceled, stopping leave operations")
			return
		default:
			leaveChannel(client, l)
			time.Sleep(delay)
		}
	}

	h.currentChannels = nextChannels
}

func (h *handler) handlePing(writer http.ResponseWriter, request *http.Request) {
	writer.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(writer,
		"Pong! Last incoming message: %v, last deleted message: %v",
		h.lastIncomingMessage, h.lastDeletedMessage)
}

func (h *handler) startCheckAndJoinTicker(client *irc.Client) {
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-h.Context.Done():
			log.Info("Context canceled, stopping checker")
			return
		case <-ticker.C:
			log.Debug("Checking for new channels to listen")

			h.checkingMu.Lock()
			h.checkAndJoinLeave(client)
			h.checkingMu.Unlock()
		}
	}
}
