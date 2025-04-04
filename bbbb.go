package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/CallumKerson/podcasts"
	"github.com/google/uuid"
	"github.com/kkdai/youtube/v2"
	"gopkg.in/yaml.v3"
)

const ytxmlurl = "https://www.youtube.com/feeds/videos.xml?channel_id="

var ytclient = youtube.Client{}

func main() {
	urls, channels, err := loadpodcastsfile("podcasts.yaml")
	if err != nil {
		log.Fatalln("could not load the podcast file: ", err)
	}

	process_shows(urls)
	create_podcast(channels)
}

// generate yaml files per yt show with the podcasts info
// downloads mp4 files into channel dirs
func process_shows(urls []string) {
	for _, item := range urls {
		// define 3x sets of items
		var uniqmp4 []Podcastitem
		ytpdcsts, filefeed, author := fromxmltoyaml(item)
		uniqmp4 = createniqueset(ytpdcsts, filefeed)
		writeshowyaml(author, &uniqmp4)
		for _, item := range uniqmp4 {
			dlmp4(ytclient, &item.Video)
		}
	}
}

// helpers after this
func create_podcast(items Channels2follow) {
	for _, item := range items.Ytchannels {
		var dirname string
		if strings.Contains(item.Name, " ") {
			dirname = strings.Replace(item.Name, " ", "", -1)
		}
		p := podcasts.Podcast{
			Title:       item.Name,
			Description: item.Description,
			Link:        item.Link + "/" + dirname + ".xml",
			Language:    item.Language,
		}

		chapters, err := readshowyaml(item.Name)
		if err != nil {
			log.Fatalln(err)
		}
		for _, chapter := range chapters {
			pubdate, err := time.Parse(time.RFC3339, chapter.Published)
			if err != nil {
				log.Fatalln(err)
			}
			cdata := new(podcasts.CDATAText)
			cdata.Value = chapter.Video.Description
			p.AddItem(&podcasts.Item{
				Title:       chapter.Title,
				Description: cdata,
				GUID:        "http://whatever.example.com/" + chapter.Guid,
				PubDate: &podcasts.PubDate{
					Time: pubdate},
				Author: item.Name,
				Duration: &podcasts.Duration{
					Duration: chapter.Duration,
				},
				Enclosure: &podcasts.Enclosure{
					URL:    item.Link + "/" + chapter.Mp4file + ".mp3",
					Length: chapter.Video.Formats[0].ApproxDurationMs,
					Type:   "audio/x-mpeg",
				},
				Image: &podcasts.ItunesImage{
					//Href: chapter.Video.Thumbnails[0].URL + ".jpg",
					Href: "https://www.shutterstock.com/shutterstock/photos/2355208533/display_1500/stock-vector-vector-illustration-of-the-black-and-white-mandala-size-x-px-the-idea-for-design-products-2355208533.jpg",
				},
			})
		}
		feed, err := p.Feed(
			podcasts.Author(item.Name),
			podcasts.Block,
			podcasts.Complete,
			podcasts.NewFeedURL(item.Link+"/"+dirname+".xml"),
			podcasts.Subtitle(item.Description),
			podcasts.Summary(item.Description),
		)
		if err != nil {
			log.Fatal(err)
		}
		fh, err := os.OpenFile(dirname+".xml", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			log.Fatalln(err)
		}
		defer fh.Close()
		err = feed.Write(fh)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

// process the xml in the url, return 2 sets of Podcastitem strucs, and the
// author name
// the 1st set comes from the xml, the second set is from the written yaml file
// we generate, so we only save info once later
func fromxmltoyaml(url string) ([]Podcastitem, []Podcastitem, string) {
	ytfeed, err := loadxml(url)
	if err != nil {
		log.Fatalln("could not load the xml for ", ytfeed.Author.Name, ": ", err)
	}
	filefeed, err := readshowyaml(ytfeed.Author.Name)
	if err != nil {
		log.Fatalln("could not read ", ytfeed.Author.Name, err)
	}
	return ytpodcastitems(ytfeed), filefeed, ytfeed.Author.Name

}

// merge the sets from xml and from file, return just one set of unique
// []Podcastitem
func createniqueset(fromyt, fromfile []Podcastitem) (uniqmp4 []Podcastitem) {
	var pdcsts, uniq []Podcastitem
	pdcsts = append(pdcsts, fromfile...)
	pdcsts = append(pdcsts, fromyt...)

	keys := make(map[string]bool)

	for _, item := range pdcsts {
		if _, value := keys[item.Published]; !value {
			keys[item.Published] = true
			uniq = append(uniq, item)
		}
	}
	for _, item := range uniq {
		addmp4link(&item)
		uniqmp4 = append(uniqmp4, item)
	}
	noshorts := removeshorts(&uniqmp4)
	//return uniqmp4
	return noshorts
}

func removeshorts(pdcsts *[]Podcastitem) []Podcastitem {
	var longerthan1m []Podcastitem
	var min time.Duration
	min = 61 * time.Second
	for _, item := range *pdcsts {
		if item.Video.Duration <= min {
			continue
		} else {
			addguid(&item)
			longerthan1m = append(longerthan1m, item)
		}
	}
	return longerthan1m
}

// fill in mp4 fields of Podcastitem; if name if video starts with '-' replace it
// or cli for ffmpeg fails - it thinks it's a ffmpeg cli switch
// skip titles with #short tag and videos whose duration is 0s
func addmp4link(item *Podcastitem) bool {
	var err error
	if strings.Contains(item.Title, "#shorts") {
		return false
	}
	if strings.HasPrefix(item.Mp4file, "-") {
		item.Mp4file = strings.Replace(item.Mp4file, "-", "_", 1)
		log.Println("do we have this file starting with _: ", item.Mp4file, ".mp3")
		_, err = os.Open(item.Mp4file + ".mp3")
	} else {
		_, err = os.Open(item.Mp4file + ".mp3")
	}
	if errors.Is(err, os.ErrNotExist) {
		log.Println(item.Title, " ", item.Duration, "s not available, keep running")
	} else {
		return true
	}
	video, err := _getsmallessvideo(item.Link, ytclient)
	if err != nil {
		premiere, kk := regexp.MatchString("LIVE_STREAM_OFFLINE", err.Error())
		if kk != nil {
			log.Println("error matching LIVE_STREAM_OFFLINE: ", kk)
		}
		if premiere == true {
			log.Println("episode not yet published, waiting to be live streamed")
			return true
		}
	}
	if strings.HasPrefix(video.ID, "-") {
		item.Mp4file = strings.Replace(video.ID, "-", "_", 1)
	} else {
		item.Mp4file = video.ID
	}
	item.Video = *video
	item.Duration = video.Duration.Abs()
	return true
}

func addguid(item *Podcastitem) {
	err := uuid.Validate(item.Guid)
	if err != nil {
		fmt.Println("Guid is not valid, probably new episode: ", err)
		item.Guid = uuid.New().String()
	}
}

// retrieve the videos with audio only, then the one with the smalles footprint
func _getsmallessvideo(videourl string, ytclient youtube.Client) (*youtube.Video, error) {
	video, err := ytclient.GetVideo(videourl)
	if err != nil {
		premiere, kk := regexp.MatchString("LIVE_STREAM_OFFLINE", err.Error())
		if kk != nil {
			log.Println("error matching LIVE_STREAM_OFFLINE: ", kk)
		}
		if premiere == true {
			log.Println(videourl, " episode not yet published, waiting to be live streamed")
			return nil, err
		} else {
			log.Println("Error while getting smallest video", videourl, err.Error())
			return nil, err
		}
	}
	tiny := (video.Formats.WithAudioChannels().Quality("tiny"))
	smallest := tiny[0]

	for _, f := range tiny {
		if f.ContentLength < smallest.ContentLength {
			smallest = f
		}
	}
	// empty the formats list, add only the smallest one, not interested in the
	// rest - yaml file gets huge otherwise
	video.Formats = nil
	video.Formats = append(video.Formats, smallest)
	return video, nil

}

// load podcasts.yaml, return the channel id urls
func loadpodcastsfile(file string) (urls []string, podcasts Channels2follow, err error) {
	pdcfile, err := os.ReadFile(file)
	if err != nil {
		log.Fatalln("could not read ", file, err)
	}

	err = yaml.Unmarshal(pdcfile, &podcasts)
	if err != nil {
		log.Fatalln("could not unmarshal ", file, err)
	}

	for _, value := range podcasts.Ytchannels {
		urls = append(urls, ytxmlurl+value.Channelid)
	}
	return urls, podcasts, nil
}

func loadxml(url string) (feed Feed, err error) {
	var result Feed
	log.Println(url)
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
		//fmt.Println(feed.Entry[i].Title)
		item := Podcastitem{
			Title:     feed.Entry[i].Title,
			Published: feed.Entry[i].Published,
			Link:      feed.Entry[i].Link.Href,
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
	//fmt.Println(f.Name())
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

// create files, copy the yt stream to those files, set the lastmodified
// timestamps on the created files, skip already downloaded files
// removes mp4 files
func dlmp4(ytclient youtube.Client, video *youtube.Video) (videofile *os.File, err error) {
	if strings.HasPrefix(video.ID, "-") {
		video.ID = strings.Replace(video.ID, "-", "_", 1)
	}
	mp3, err := os.Open(video.ID + ".mp3")
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println(video.Title, ".mp3 not available, keep running")
	} else {
		//log.Println("mp3 already available, skipping")
		_, err := os.Stat(video.ID + ".mp4")
		if err == nil {
			err = os.Remove(video.ID + ".mp4")
			if err != nil {
				log.Println("could not remove mp4 file: ", err)
			}
		} else if os.IsNotExist(err) {
		}
		return mp3, nil
	}

	// replace prefix
	if strings.HasPrefix(video.ID, "-") {
		video.ID = strings.Replace(video.ID, "-", "_", 1)
		log.Println(video.ID)
	}
	file, err := processmp4(ytclient, video)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return file, err
}

func processmp4(ytclient youtube.Client, video *youtube.Video) (file *os.File, err error) {
	file, err = os.Open(video.ID + ".mp4")
	if err != nil && os.IsNotExist(err) {
		fh, err := os.Create(video.ID + ".mp4")
		if err != nil {
			log.Println("error creating file: ", err)
			return nil, err
		}
		defer fh.Close()
		stream, _, err := ytclient.GetStream(video, &video.Formats[0])
		if err != nil {
			forbidden, regexerr := regexp.MatchString("unexpected status code: 403", err.Error())
			if regexerr != nil {
				log.Println("error matching 403403403: ", regexerr)
			}
			if forbidden == true {
				log.Println("forbidden, retrying...")
				stream, _, err = ytclient.GetStream(video, &video.Formats[0])
			}
		}
		if err != nil {
			log.Fatalln("error streaming ", err)
			err = os.Remove(video.ID + ".mp4")
			if err != nil {
				log.Fatalln("could not remove mp4 file", err)
			}
			return nil, err
		}
		_, err = io.Copy(fh, stream)
		if err != nil {
			log.Println("error copying stream to file", err)
			matched, regexerr := regexp.MatchString("403", err.Error())
			if regexerr != nil {
				log.Println("error matching regex 403", err)
			}
			if matched == true {
				err = os.Remove(video.ID + ".mp4")
				if err != nil {
					log.Println("could not remove mp4 file", err)
					return nil, err
				}
				log.Println("got a 403 so quit")
				return nil, err

			}
		}
		log.Printf("%s copied from stream\n", fh.Name())
		log.Println("File name without ext: ", video.ID)
		err = _conver2mp3(video.ID+".mp4", video.ID+".mp3")
		if err != nil {
			log.Println("error converting to mp3", err)
			err = os.Remove(video.ID + ".mp4")
			if err != nil {
				log.Println("could not remove mp4 file: ", err)
			}
		}
	}
	return file, err
}

// convert mp4 to mp3 using ffmpeg
func _conver2mp3(mp4file, mp3file string) error {
	args := []string{"-i", mp4file, "-map", "0:a:0", "-b:a", "48k", mp3file}
	cmd := exec.Command("ffmpeg", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg conversion failed: %s", err)
	}
	f, err := os.Stat(mp3file)
	if err != nil {
		log.Fatalln(err)
	}
	if f.Size() < int64(4000000) {
		log.Printf("%s seems too small: %d, wrong conversion?\n", f.Name(), f.Size())
		log.Println("re-running ffmpeg again ....")
		_conver2mp3(mp4file, mp3file)
	}
	return nil
}

type Podcastitem struct {
	Title     string        `yaml:"title,omitempty"`
	Published string        `yaml:"published,omitempty"`
	Link      string        `yaml:"link,omitempty"`
	Guid      string        `yaml:"guid,omitempty"`
	Mp4file   string        `yaml:"mp4file,omitempty"`
	Duration  time.Duration `yaml:"duration,omitempty"`
	Video     youtube.Video `yaml:"video,omitempty"`
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
		Name        string `yaml:"name"`
		Channelid   string `yaml:"channelid"`
		Link        string `yaml:"link"`
		Language    string `yaml:"language,omitempty"`
		Description string `yaml:"description,omitempty"`
	} `yaml:"ytchannels"`
}
