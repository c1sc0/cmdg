// tool to "tail" a gmail account as email comes in.
package main

/*
 *  Copyright (C) 2015 Thomas Habets <thomas@habets.se>
 *
 *  This program is free software; you can redistribute it and/or modify
 *  it under the terms of the GNU General Public License as published by
 *  the Free Software Foundation; either version 2 of the License, or
 *  (at your option) any later version.
 *
 *  This program is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU General Public License for more details.
 *
 *  You should have received a copy of the GNU General Public License along
 *  with this program; if not, write to the Free Software Foundation, Inc.,
 *  51 Franklin Street, Fifth Floor, Boston, MA 02110-1301 USA.
 */

import (
	"flag"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/ThomasHabets/cmdg/cmdglib"
	"github.com/ThomasHabets/drive-du/lib"
	"github.com/golang/glog"
	gmail "google.golang.org/api/gmail/v1"
)

const (
	scope      = "https://www.googleapis.com/auth/gmail.readonly"
	accessType = "offline"
	email      = "me"
	pageSize   = 20
)

var (
	config       = flag.String("config", "", "Config file. If empty will default to ~/.cmdg/cmdg.conf.")
	pollInterval = flag.Duration("poll", 10*time.Second, "Time to wait between polls.")
)

func mailTail(g *gmail.Service, historyID uint64) uint64 {
	pageToken := ""
	for {
		res, err := g.Users.History.List(email).MaxResults(pageSize).StartHistoryId(historyID).PageToken(pageToken).Do()
		if err != nil {
			glog.Errorf("Listing history since %v: %v", historyID, err)
			continue
		}
		pageToken = res.NextPageToken
		for _, h := range res.History {
			var wg sync.WaitGroup
			msgs := make([]*gmail.Message, len(h.MessagesAdded), len(h.MessagesAdded))
			wg.Add(len(h.MessagesAdded))
			for n, m := range h.MessagesAdded {
				n, m := n, m
				go func() {
					defer wg.Done()
					mres, err := g.Users.Messages.Get(email, m.Message.Id).Format("full").Do()
					if err != nil {
						glog.Errorf("Getting message %q, skipping: %v", m.Message.Id, err)
					} else {
						msgs[n] = mres
					}
				}()
			}
			wg.Wait()
			for _, m := range msgs {
				if m == nil {
					continue
				}
				tss := "Unknown"
				if ts, err := cmdglib.ParseTime(cmdglib.GetHeader(m, "Date")); err == nil {
					tss = ts.Format("2006-01-02 15:04")
				}
				tsWidth := 16
				fromMax := 20
				fmt.Printf("%*.*s %*.*q: %q\n",
					tsWidth, tsWidth, tss,
					fromMax, fromMax, cmdglib.GetHeader(m, "From"),
					cmdglib.GetHeader(m, "Subject"),
				)
			}
		}
		if pageToken == "" {
			return res.HistoryId
		}
	}
}

func main() {
	flag.Parse()
	if flag.NArg() > 0 {
		glog.Exitf("Non-argument options provided: %q", flag.Args())
	}

	glog.Infof("Starting up")
	if *config == "" {
		*config = path.Join(os.Getenv("HOME"), ".cmdg", "cmdg.conf")
	}

	if fi, err := os.Stat(*config); err != nil {
		glog.Exitf("Missing config file %q: %v", *config, err)
	} else if (fi.Mode() & 0477) != 0400 {
		glog.Exitf("Config file (%q) permissions must be 0600 or better, was 0%o", *config, fi.Mode()&os.ModePerm)
	}

	conf, err := lib.ReadConfig(*config)
	if err != nil {
		glog.Exitf("Failed to read config: %v", err)
	}

	t, err := lib.Connect(conf.OAuth, scope, accessType)
	if err != nil {
		glog.Exitf("Failed to connect to gmail: %v", err)
	}
	g, err := gmail.New(t)
	if err != nil {
		glog.Exitf("Failed to create gmail client: %v", err)
	}

	// Make sure oauth keys are correct before setting up ncurses.
	prof, err := g.Users.GetProfile(email).Do()
	if err != nil {
		glog.Exitf("Get profile: %v", err)
	}
	historyID := prof.HistoryId

	if false {
		initialMessages := int64(10)
		res, err := g.Users.Messages.List(email).MaxResults(initialMessages).Do()
		if err != nil {
			glog.Exitf("Getting messages: %v", err)
		}
		msg := res.Messages[len(res.Messages)-1]
		m, err := g.Users.Messages.Get(email, msg.Id).Format("full").Do()
		if err != nil {
			glog.Errorf("Getting latest message %q: %v", msg.Id, err)
		} else {
			historyID = m.HistoryId
		}
	}
	for {
		historyID = mailTail(g, historyID)
		time.Sleep(*pollInterval)
	}
}
