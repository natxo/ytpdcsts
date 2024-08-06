package main

import (
	"encoding/xml"
	"os"
	"slices"
	"testing"
)

func TestLoadpodcastbfile(t *testing.T) {
	exp_ids := []string{"UCsLiV4WJfkTEHH0b9PmRklw", "UC-tLyAaPbRZiYrOJxAGB7dQ", "UC2PA-AKmVpU6NKCGtZq_rKQ"}
	var exp_urls []string
	for _, id := range exp_ids {
		exp_urls = append(exp_urls, ytxmlurl+id)
	}
	var urls []string
	var channels2follow Channels2follow
	var channel_ids []string
	var err error
	t.Run("load podcast file and generate urls", func(t *testing.T) {
		urls, channels2follow, err = loadpodcastsfile("podcasts.yaml.sample")
		if err != nil {
			t.Fatalf("something went wrong reading the podcasts.yaml.sample file")
		}
		if !slices.Equal(urls, exp_urls) {
			t.Errorf("expectedd %s, got %s instead", urls, exp_urls)
		}
	})
	t.Run("get channel ids", func(t *testing.T) {
		for _, channel := range channels2follow.Ytchannels {
			channel_ids = append(channel_ids, channel.Channelid)
		}

		if !slices.Equal(channel_ids, exp_ids) {
			t.Errorf("expectedd %s, got %s instead", channel_ids, exp_ids)
		}
	})

}

func TestParsexmlfeed(t *testing.T) {
	var feed Feed
	t.Run("read xml feed file", func(t *testing.T) {
		fh, err := os.ReadFile("testdata/webdrivertorso.xml")
		if err != nil {
			t.Errorf("could not read feedxml file: %s", err)
		}
		err = xml.Unmarshal(fh, &feed)
		if err != nil {
			t.Errorf("error unmarshalling feed file %s", err)
		}
	})

}
