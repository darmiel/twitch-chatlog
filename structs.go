package main

import (
	"fmt"
	"gopkg.in/irc.v3"
	"strconv"
	"time"
)

///

type User struct {
	ID   int64 `gorm:"primaryKey"`
	Name string
}

type Channel struct {
	ID   int64 `gorm:"primaryKey"`
	Name string
}

type Message struct {
	ID   string    `gorm:"primaryKey"`
	Body string    `gorm:"not null"`
	Mod  bool      `gorm:"not null"`
	Date time.Time `gorm:"not null"`

	ChannelID int64 `gorm:"not null"`
	AuthorID  int64 `gorm:"not null"`

	// Belongings
	Author  *User
	Channel *Channel
}

type ListeningChannel struct {
	ChannelName string `gorm:"primaryKey"`
	Active      bool   `gorm:"default:true"`
}

////

// ParseIRCMessage expects {msg} to have {Command == PRIVMSG}
func ParseIRCMessage(msg *irc.Message) (*Message, error) {
	// meta
	var (
		userName    = msg.Name
		channelName = msg.Param(0)
		body        = msg.Param(1)
		messageID   string
		userID      int64
		channelID   int64
		isMod       bool
	)
	if len(channelName) > 0 {
		channelName = channelName[1:]
	}

	// auxiliary
	var (
		useridstr    string
		channelidstr string
		modstr       string
		ok           bool
		err          error
	)

	if useridstr, ok = msg.GetTag("user-id"); !ok {
		return nil, fmt.Errorf("userid was empty for user %v", userName)
	}
	if userID, err = strconv.ParseInt(useridstr, 10, 64); err != nil {
		return nil, fmt.Errorf("userid %v could not be converted to int64", useridstr)
	}

	if channelidstr, ok = msg.GetTag("room-id"); !ok {
		return nil, fmt.Errorf("roomid was empty for channel %v", channelName)
	}
	if channelID, err = strconv.ParseInt(channelidstr, 10, 64); err != nil {
		return nil, fmt.Errorf("roomid %v could not be converted to int64", channelidstr)
	}

	if messageID, ok = msg.GetTag("id"); !ok {
		return nil, fmt.Errorf("messageid was empty for body '%v' by %v", body, userName)
	}

	if modstr, ok = msg.GetTag("mod"); !ok {
		return nil, fmt.Errorf("mod was empty for body '%v' by %v", body, userName)
	}
	isMod = modstr == "1"

	if isMod {
		return nil, fmt.Errorf("test error")
	}

	return &Message{
		ID:       messageID,
		Body:     body,
		Mod:      isMod,
		Date:     time.Now(),
		AuthorID: userID,
		Author: &User{
			ID:   userID,
			Name: userName,
		},
		ChannelID: channelID,
		Channel: &Channel{
			ID:   channelID,
			Name: channelName,
		},
	}, nil
}
