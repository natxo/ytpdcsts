package main

import (
	"slices"
	"testing"
)

// need to test the channels too
func TestLoadpodcastbfile(t *testing.T) {
	exp_urls := []string{ytxmlurl + "UC-tLyAaPbRZiYrOJxAGB7dQ", ytxmlurl + "UC2PA-AKmVpU6NKCGtZq_rKQ"}
	//exp_channels := []string("UC-tLyAaPbRZiYrOJxAGB7dQ", "UC2PA-AKmVpU6NKCGtZq_rKQ")

	urls, _, err := loadpodcastsfile("podcasts.yaml.sample")
	if err != nil {
		t.Errorf("something went wrong reading the podcasts.yaml.sample file")
	}
	if !slices.Equal(urls, exp_urls) {
		t.Errorf("expectedd %s, got %s instead", urls, exp_urls)
	}

}
