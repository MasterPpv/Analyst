package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"github.com/garyburd/go-oauth/oauth"
	"github.com/garyburd/twitterstream"
	"github.com/ChimeraCoder/anaconda"
	"github.com/nsf/termbox-go"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	configPath = flag.String("config", "config.json", "Path to configuration file containing the application's credentials.")

	accessToken oauth.Credentials

	oauthClient = oauth.Client {
		TemporaryCredentialRequestURI: "https://api.twitter.com/oauth/request_token",
		ResourceOwnerAuthorizationURI: "https://api.twitter.com/oauth/authorize",
		TokenRequestURI:               "https://api.twitter.com/oauth/access_token",
	}
)

func readConfig() error {
	b, err := ioutil.ReadFile(*configPath)
	if err != nil {
		return err
	}
	var config = struct {
		Consumer, Access *oauth.Credentials
	}{
		&oauthClient.Credentials, &accessToken,
	}
	return json.Unmarshal(b, &config)
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n %s keyword ...\n", os.Args[0], os.Args[0])
	flag.PrintDefaults()
	os.Exit(2)
}

func termboxQuery() string {
	// Initializes the termbox
	termbox_err := termbox.Init()
	if termbox_err != nil {
		panic(termbox_err)
	}
	defer termbox.Close()
	termbox.SetInputMode(termbox.InputEsc)
	edit_box.InsertRune('#')

	// Queries the user for a hashtag to search
	redraw_all()
	query := ""
	stringSlices := make([]rune, 0)
	stringSlices = append(stringSlices, '#')
	queryloop: for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyEsc:
				// If Esc is pressed, abort query
				break queryloop
			case termbox.KeySpace:
				// If space is pressed, add a space
				edit_box.InsertRune(' ')
				stringSlices = append(stringSlices, ' ')
			case termbox.KeyEnter:
				// If Enter pressed, use query 
				query = string(stringSlices)
				break queryloop
			case termbox.KeyBackspace, termbox.KeyBackspace2:
				// If Backspace is pressed, delete a letter
				edit_box.DeleteRuneBackward()
				if len(stringSlices) > 0 {
					tempSlices := make([]rune, len(stringSlices) - 1)
					copy(tempSlices, stringSlices[0:])
					stringSlices[len(stringSlices) - 1] = 0
					stringSlices = tempSlices
				}
			default:
				// If any other key is pressed, add the
				// corresponding letter to the query string
				if ev.Ch != 0 {
					edit_box.InsertRune(ev.Ch)
					stringSlices = append(stringSlices, ev.Ch)
				}
			}
		case termbox.EventError:
			panic(ev.Err)
		}
		redraw_all()
	}
	if query == "" || query == "#" || query == " " || query == "# " {
		fmt.Println("No query given. Nothing to search for. Program exiting...")
		return query
	} else {
		termbox.Close()
		fmt.Print("Searching for: ", query, "\n")
		fmt.Println()
		return query
	}
}

func main() {
	// Query the user for a search entry
	searchQuery := termboxQuery()
	// Deal with the empty cases
	if searchQuery == "" || searchQuery == "#" || searchQuery == " " || searchQuery == "# " {
		return
	}
	searchTweetCount := 0

	// Query for the second search entry
	compareQuery := termboxQuery()
	if compareQuery == "" || compareQuery == "#" || compareQuery == " " || compareQuery == "# " {
		return
	}
	compareTweetCount := 0

	// Read OAuth credentials from config file
	flag.Usage = usage
	flag.Parse()
	if err := readConfig(); err != nil {
		log.Fatalf("Error reading configuration, %v", err)
	}

	// Connect to the default MongoDB session
	mdbSession, mdbErr := mgo.Dial("127.0.0.1:27017")
	if mdbErr != nil {
		panic(mdbErr)
	}
	defer mdbSession.Close()

	// Open a Twitter stream using OAuth credentials and start
	// listening for Tweets containing the user's query
	ts, tsErr := twitterstream.Open(
		&oauthClient,
		&accessToken,
		"https://stream.twitter.com/1.1/statuses/filter.json",
		url.Values{"track": {searchQuery}})
	if tsErr != nil {
		log.Fatal(tsErr)
	}
	defer ts.Close()

	cs, csErr := twitterstream.Open(
		&oauthClient,
		&accessToken,
		"https://stream.twitter.com/1.1/statuses/filter.json",
		url.Values{"track": {compareQuery}})
	if csErr != nil {
		log.Fatal(csErr)
	}
	defer cs.Close()

	queryTweetCollection := mdbSession.DB("test").C("QueryTweets")
	compareTweetCollection := mdbSession.DB("test").C("CompareTweets")
	// Loop until stream has a permanent error.
	for ts.Err() == nil && cs.Err() == nil {
		// As each tweet comes in, mine it for data and print it
		var qt anaconda.Tweet
		var ct anaconda.Tweet
		if queryTweetErr := ts.UnmarshalNext(&qt); queryTweetErr != nil {
			log.Fatal(queryTweetErr)
		}
		if compareTweetErr := cs.UnmarshalNext(&ct); compareTweetErr != nil {
			log.Fatal(compareTweetErr)
		}
		fmt.Print("Username: @", qt.User.ScreenName, "\n")
		fmt.Print("Tweet: ", qt.Text, "\n")
		fmt.Print("URL: https://twitter.com/", qt.User.ScreenName, "/status/", qt.Id, "\n")
		queryTweetTime, queryTimeErr := qt.CreatedAtTime()
		if queryTimeErr != nil {
			panic(queryTimeErr)
		}
		fmt.Print("Time created: ", queryTweetTime.String())
		fmt.Println()
		fmt.Println()
		searchTweetCount += 1
		if searchTweetCount == 25 {
			ts.Close()
		}
		fmt.Print("Username: @", ct.User.ScreenName, "\n")
		fmt.Print("Tweet: ", ct.Text, "\n")
		fmt.Print("URL: https://twitter.com/", ct.User.ScreenName, "/status/", ct.Id, "\n")
		compareTweetTime, compareTimeErr := ct.CreatedAtTime()
		if compareTimeErr != nil {
			panic(compareTimeErr)
		}
		fmt.Print("Time created: ", compareTweetTime.String())
		fmt.Println()
		fmt.Println()
		compareTweetCount += 1
		if compareTweetCount == 25 {
			cs.Close()
		}
		tweetM := bson.M {
			"ScreenName":qt.User.ScreenName,
			"Text":qt.Text,
			"Id":qt.Id,
			"CreatedAt":queryTweetTime,
		}
		queryInsertErr := queryTweetCollection.Insert(&tweetM)
		if queryInsertErr != nil {
			log.Fatal(queryInsertErr)
		}
		compareM := bson.M {
			"ScreenName":ct.User.ScreenName,
			"Text":ct.Text,
			"Id":ct.Id,
			"CreatedAt":compareTweetTime,
		}
		compareInsertErr := compareTweetCollection.Insert(&compareM)
		if compareInsertErr != nil {
			log.Fatal(compareInsertErr)
		}
	}
	if ts.Err != nil {
		log.Print(ts.Err)
	}
	if cs.Err != nil {
		log.Print(cs.Err)
	}

	// Analyze data from database here

}