package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const PORT = 42069
const COPY_RETRIES = 12
const DELETE_RETRIES = 12
const COPY_RETRY_DELAY = time.Second * 5
const DELETE_RETRY_DELAY = time.Second * 5
const DEFAULT_CALENDAR_ID = "primary"

var codeChannel = make(chan string, 2)

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	tok := getTokenFromWeb(config)
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("%v\n", authURL)

	authCode := <-codeChannel

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %+v", err)
	}
	return tok
}

func getNextPage(srv *calendar.Service, nextPageToken string) *calendar.Events {
	events, err := srv.Events.List(DEFAULT_CALENDAR_ID).ShowDeleted(false).
		SingleEvents(true).PageToken(nextPageToken).OrderBy("startTime").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve user's events: %+v", err)
	}
	return events
}

func getEvents(srv *calendar.Service) []*calendar.Event {
	events := getNextPage(srv, "")
	items := events.Items
	for events.NextPageToken != "" {
		events = getNextPage(srv, events.NextPageToken)
		items = append(items, events.Items...)
	}
	return items
}

func makeServiceWithScopes(ctx context.Context, jsonkey []byte, scope ...string) *calendar.Service {
	config, err := google.ConfigFromJSON(jsonkey, scope...)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %+v", err)
	}
	client := getClient(config)

	srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Calendar client: %+v", err)
	}
	return srv
}

func serveCode(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	codeChannel <- code
	w.Write([]byte("You can now close this tab."))
}

func copyEvent(srv *calendar.Service, event *calendar.Event) bool {
	var err error
	var _ *calendar.Event
	c := *event
	c.Id = ""
	c.Creator = nil
	c.Etag = ""
	c.HangoutLink = ""
	c.HtmlLink = ""
	c.ICalUID = ""
	c.Organizer = nil
	c.RecurringEventId = ""
	c.Attendees = make([]*calendar.EventAttendee, 0, 1)
	if c.Reminders.UseDefault {
		c.Reminders.Overrides = make([]*calendar.EventReminder, 0, 1)
	}
	for i := 0; i < COPY_RETRIES; i++ {
		_, err = srv.Events.Insert(DEFAULT_CALENDAR_ID, &c).Do()
		if err != nil {
			log.Printf("Error copying event: %+v\n", err)
			time.Sleep(COPY_RETRY_DELAY)
		} else {
			return true
		}
	}
	log.Printf("Error copying event: %+v\n", err)
	return false
}

func deleteEvent(srv *calendar.Service, event *calendar.Event) bool {
	var err error
	for i := 0; i < DELETE_RETRIES; i++ {
		err = srv.Events.Delete(DEFAULT_CALENDAR_ID, event.Id).Do()
		if err != nil {
			log.Printf("Error deleting event: %+v\n", err)
			time.Sleep(DELETE_RETRY_DELAY)
		} else {
			return true
		}
	}
	log.Printf("Error deleting event: %+v\n", err)
	return false
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", serveCode)

	go func() {
		http.ListenAndServe(fmt.Sprintf(":%d", PORT), r)
	}()

	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	fmt.Println("Please navigate to the following link and authorize with the account that you want to move events *from*.")
	sourceSrv := makeServiceWithScopes(
		ctx, b, calendar.CalendarReadonlyScope, calendar.CalendarEventsScope)
	fmt.Println("Please navigate to the following link and authorize with the account that you want to move events *to*.")
	destSrv := makeServiceWithScopes(
		ctx, b, calendar.CalendarReadonlyScope, calendar.CalendarEventsScope)

	copyFailed := make([]*calendar.Event, 0, 10)
	deleteFailed := make([]*calendar.Event, 0, 10)
	items := getEvents(sourceSrv)
	fmt.Printf("Found %d events.\n", len(items))
	var success = true
	for i, item := range items {
		success = copyEvent(destSrv, item)
		if !success {
			copyFailed = append(copyFailed, item)
			deleteFailed = append(deleteFailed, item)
		} else {
			success = deleteEvent(sourceSrv, item)
			if !success {
				deleteFailed = append(deleteFailed, item)
			}
		}

		log.Printf("%d/%d (%.2f%%)\n", i+1, len(items), 100.0*float64(i+1)/float64(len(items)))
	}
	if len(copyFailed) > 0 {
		fmt.Printf("%d items failed to copy:\n", len(copyFailed))
	}
	for _, event := range copyFailed {
		fmt.Printf("%s\t%s\n", event.Start.DateTime, event.Start.Date)
	}
	if len(deleteFailed) > 0 {
		fmt.Printf("%d items failed to delete:\n", len(deleteFailed))
	}
	for _, event := range deleteFailed {
		fmt.Printf("%s\t%s\n", event.Start.DateTime, event.Start.Date)
	}
}
