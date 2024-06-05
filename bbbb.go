package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"

	"github.com/google/uuid"
	"github.com/kkdai/youtube/v2"
	"gopkg.in/yaml.v3"
)

const ytxmlurl = "https://www.youtube.com/feeds/videos.xml?channel_id="

var ytclient = youtube.Client{}

func main() {
	urls, err := loadpodcastsfile("podcasts.yaml")
	if err != nil {
		log.Fatalln("could not load the podcast file: ", err)
	}

	process_shows(urls)
}

// generate yaml files per yt show with the podcasts info
func process_shows(urls []string) {
	for _, item := range urls {
		var pdcsts []Podcastitem
		ytfeed, err := loadxml(item)
		if err != nil {
			log.Fatalln("could not load the xml for ", ytfeed.Author.Name, ": ", err)
		}
		ytpdcsts := ytpodcastitems(ytfeed)

		filefeed, err := readshowyaml(ytfeed.Author.Name)
		if err != nil {
			log.Fatalln("could not read ", ytfeed.Author.Name, err)
		}
		pdcsts = append(pdcsts, filefeed...)
		pdcsts = append(pdcsts, ytpdcsts...)

		keys := make(map[string]bool)
		var uniq, uniqmp4 []Podcastitem

		for _, item := range pdcsts {
			if _, value := keys[item.Published]; !value {
				keys[item.Published] = true
				uniq = append(uniq, item)
			}
		}
		for _, item := range uniq {
			addmp4link(&item)
			addguid(&item)
			uniqmp4 = append(uniqmp4, item)
		}

		writeshowyaml(ytfeed.Author.Name, &uniqmp4)
	}
}

func addmp4link(item *Podcastitem) {
	if len(item.Link) > 0 && len(item.Mp4) == 0 {
		//video, format, err := _getsmallessvideo(item.Link, ytclient)
		_, format, err := _getsmallessvideo(item.Link, ytclient)
		if err != nil {
			log.Fatalln("error while getting smallest video: ", err)
		}
		item.Mp4 = format.URL
	}
}

func addguid(item *Podcastitem) {
	err := uuid.Validate(item.Guid)
	if err != nil {
		fmt.Println("Guid is not valid!", err)
		item.Guid = uuid.New().String()
	} else {
		fmt.Println("Guid is valid!")
	}
}

// retrieve the videos with audio only, then the one with the smalles footprint
func _getsmallessvideo(videourl string, ytclient youtube.Client) (*youtube.Video, youtube.Format, error) {
	video, err := ytclient.GetVideo(videourl)
	if err != nil {
		premiere, kk := regexp.MatchString(`LIVE_STREAM_OFFLINE`, err.Error())
		if kk != nil {
			log.Fatalln("error matching string: ", kk)
		}
		if premiere == true {
			log.Println(videourl, " episode not yet published, waiting to be live streamed")
			var dummy youtube.Format
			return nil, dummy, err
		} else {
			log.Println(err.Error())
			return nil, video.Formats[0], err
		}
	}
	tiny := (video.Formats.WithAudioChannels().Quality("tiny"))
	smallest := tiny[0]
	for _, f := range tiny {
		if f.ContentLength < smallest.ContentLength {
			smallest = f
		}
	}
	return video, smallest, nil

}

// load podcasts.yaml, return the channel id urls
func loadpodcastsfile(file string) (urls []string, err error) {
	pdcfile, err := os.ReadFile("podcasts.yaml")
	if err != nil {
		panic("could not read podcasts.yaml")
	}

	var podcasts Channels2follow

	err = yaml.Unmarshal(pdcfile, &podcasts)
	if err != nil {
		panic("could not unmarshal podcasts.yaml")
	}

	for _, value := range podcasts.Ytchannels {
		urls = append(urls, ytxmlurl+value.Channelid)
	}
	return urls, nil
}

func loadxml(url string) (feed Feed, err error) {
	var result Feed
	fmt.Println(url)
	if xmlBytes, err := getXML(url); err != nil {
		log.Fatalf("Failed to get XML: %v", err)
	} else {
		err := xml.Unmarshal(xmlBytes, &result)
		if err != nil {
			return result, err
		}
		return result, nil
	}
	return result, nil
}

// load xml doc from url, returns []byte and error
func getXML(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return []byte{}, fmt.Errorf("GET error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []byte{}, fmt.Errorf("Status error: %v", resp.StatusCode)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, fmt.Errorf("Read body: %v", err)
	}

	return data, nil
}

// load show yaml into []Podcastime list
func readshowyaml(show string) (pdcs []Podcastitem, err error) {
	f, err := os.ReadFile(show + ".yaml")
	if err != nil {
		if os.IsNotExist(err) {
			os.Create(show + ".yaml")
		} else {
			log.Fatalln("could not read file ", show+".yaml", err)
		}
	}
	err = yaml.Unmarshal(f, &pdcs)

	if err != nil {
		log.Fatalln("could not unmarshal ", show+".yaml", err)
	}
	return pdcs, nil
}

// extract []Podcastitem from Feed, yt only gives us the last 15
func ytpodcastitems(feed Feed) []Podcastitem {
	var pdcstitems []Podcastitem

	for i := 0; i < len(feed.Entry); i++ {
		item := Podcastitem{
			Title:     feed.Entry[i].Title,
			Published: feed.Entry[i].Published,
			Link:      feed.Entry[i].Link.Href,
			Mp4:       "",
		}
		pdcstitems = append(pdcstitems, item)
	}
	return pdcstitems
}

// writes the feed info to a yaml file/poorman's db
// here we save the older we already go and the newest 15 from the yt xml feed
func writeshowyaml(show string, pdcsts *[]Podcastitem) error {
	f, err := os.OpenFile(show+".yaml", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalln("could not create or truncate file", show+".yaml", err)
	}
	fmt.Println(f.Name())
	defer f.Close()

	data, err := yaml.Marshal(pdcsts)
	if err != nil {
		log.Fatalln("error marshaling items into show yaml")
	}

	_, err = f.Write(data)
	if err != nil {
		log.Fatalln("error writing show yaml file")
	}
	return nil
}

type Podcastitem struct {
	Title     string `yaml:"title,omitempty"`
	Published string `yaml:"published,omitempty"`
	Link      string `yaml:"link,omitempty"`
	Mp4       string `yaml:"mp4,omitempty"`
	Guid      string `yaml:"guid,omitempty"`
}

type Feed struct {
	XMLName xml.Name `xml:"feed"`
	Text    string   `xml:",chardata"`
	Yt      string   `xml:"yt,attr"`
	Media   string   `xml:"media,attr"`
	Xmlns   string   `xml:"xmlns,attr"`
	Link    []struct {
		Text string `xml:",chardata"`
		Rel  string `xml:"rel,attr"`
		Href string `xml:"href,attr"`
	} `xml:"link"`
	ID        string `xml:"id"`
	ChannelId string `xml:"channelId"`
	Title     string `xml:"title"`
	Author    struct {
		Text string `xml:",chardata"`
		Name string `xml:"name"`
		URI  string `xml:"uri"`
	} `xml:"author"`
	Published string `xml:"published"`
	Entry     []struct {
		Text      string `xml:",chardata"`
		ID        string `xml:"id"`
		VideoId   string `xml:"videoId"`
		ChannelId string `xml:"channelId"`
		Title     string `xml:"title"`
		Link      struct {
			Text string `xml:",chardata"`
			Rel  string `xml:"rel,attr"`
			Href string `xml:"href,attr"`
		} `xml:"link"`
		Author struct {
			Text string `xml:",chardata"`
			Name string `xml:"name"`
			URI  string `xml:"uri"`
		} `xml:"author"`
		Published string `xml:"published"`
		Updated   string `xml:"updated"`
		Group     struct {
			Text    string `xml:",chardata"`
			Title   string `xml:"title"`
			Content struct {
				Text   string `xml:",chardata"`
				URL    string `xml:"url,attr"`
				Type   string `xml:"type,attr"`
				Width  string `xml:"width,attr"`
				Height string `xml:"height,attr"`
			} `xml:"content"`
			Thumbnail struct {
				Text   string `xml:",chardata"`
				URL    string `xml:"url,attr"`
				Width  string `xml:"width,attr"`
				Height string `xml:"height,attr"`
			} `xml:"thumbnail"`
			Description string `xml:"description"`
			Community   struct {
				Text       string `xml:",chardata"`
				StarRating struct {
					Text    string `xml:",chardata"`
					Count   string `xml:"count,attr"`
					Average string `xml:"average,attr"`
					Min     string `xml:"min,attr"`
					Max     string `xml:"max,attr"`
				} `xml:"starRating"`
				Statistics struct {
					Text  string `xml:",chardata"`
					Views string `xml:"views,attr"`
				} `xml:"statistics"`
			} `xml:"community"`
		} `xml:"group"`
	} `xml:"entry"`
}

type Channels2follow struct {
	Ytchannels []struct {
		Name      string `yaml:"name"`
		Channelid string `yaml:"channelid"`
		Link      string `yaml:"link"`
		Language  string `yaml:"language"`
	} `yaml:"ytchannels"`
}
