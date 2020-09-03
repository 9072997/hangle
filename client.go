package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/9072997/jgh"
	"github.com/c-bata/go-prompt"
)

const appsScriptMaxExecutionTime = time.Minute * 30

// ObjectProperty is what gets returned when we ask Apps Script to
// __DESCRIBE a variable
type ObjectProperty struct {
	Name   string `json:"name"`
	JsType string `json:"type"`
}

// seperator characters
var chainSeperators = `!@#$%^&*()_+-={}|\:;<>?,/~`
var allSeperators = `!@#$%^&*()_+-={}|[]\:";'<>?,./~` + "`"

func main() {
	toAppsScript := make(chan string)
	fromAppsScript := make(chan string)
	remoteCommand := func(command string) string {
		toAppsScript <- command
		return <-fromAppsScript
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		reqBody, err := ioutil.ReadAll(req.Body)
		jgh.PanicOnErr(err)

		if string(reqBody) != "__KEEPALIVE" {
			fromAppsScript <- string(reqBody)
		}

		select {
		case input := <-toAppsScript:
			resp.Write([]byte(input))
		case <-time.After(45 * time.Second):
			resp.Write([]byte("__USER_INPUT_TIMEOUT"))
		}

	})

	srv := http.Server{
		Addr:    ":80",
		Handler: mux,
	}
	go func() {
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			panic(err)
		}
	}()

	// file for storing command history
	currentUser, _ := user.Current()
	homeDir := currentUser.HomeDir
	var historyEntries []string
	var historyFile *os.File
	// fail silently if we couldn't open a history file
	if homeDir != "" {
		historyPath := filepath.Join(homeDir, ".hangle_history")
		// get previous entries from history file
		historyContents, err := ioutil.ReadFile(historyPath)
		// errors are fine since we will get an empty string
		if err != nil {
			fmt.Println(err)
		}
		historyEntries = strings.Split(string(historyContents), "\n")
		// open history file for appending
		historyFile, err = os.OpenFile(
			historyPath,
			os.O_CREATE|os.O_APPEND|os.O_SYNC|os.O_WRONLY,
			0644,
		)
		if err != nil {
			fmt.Println(err)
		}
	}

	// wait for "__READY" from server
	fmt.Println("Waiting for connection on port 80")
	<-fromAppsScript
	scriptEndTime := time.Now().Add(appsScriptMaxExecutionTime)

	// cache object properties. This will be reset every time we run a
	// command.
	objectPropertiesCache := make(map[string][]ObjectProperty)

	prompt.New(
		func(command string) {
			// write to history (ignore errors)
			historyFile.WriteString(command + "\n")

			if command == "exit" {
				// this will block until read by the server thread
				toAppsScript <- "__DISCONNECT"

				// wait for current requests to finish
				ctx, cancel := context.WithTimeout(
					context.Background(),
					5*time.Second,
				)
				defer cancel()
				srv.Shutdown(ctx)

				historyFile.Close()

				os.Exit(0)
			}

			response := remoteCommand(command)
			fmt.Println(response)

			// remote state changed, so clear cache of object properties
			objectPropertiesCache = make(map[string][]ObjectProperty)
		},
		func(line prompt.Document) (suggestions []prompt.Suggest) {
			chain := line.GetWordBeforeCursorUntilSeparator(chainSeperators)
			// break chain into a stable prefix the user is unlikely to
			// want completion for and an unstable part they do want
			// completion for.
			match := regexp.
				MustCompile(`^(.+)\.(.*)$`).
				FindStringSubmatch(chain)
			if match == nil {
				return
			}
			context := match[1]
			filter := match[2]

			properties, inCache := objectPropertiesCache[context]
			if !inCache {
				propsJSON := remoteCommand("__DESCRIBE " + context)
				json.Unmarshal([]byte(propsJSON), &properties)
				objectPropertiesCache[context] = properties
			}

			for _, property := range properties {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        property.Name,
					Description: property.JsType,
				})
			}

			return prompt.FilterContains(suggestions, filter, true)
		},
		prompt.OptionCompletionWordSeparator(allSeperators),
		prompt.OptionLivePrefix(func() (prefix string, useLivePrefix bool) {
			remaining := scriptEndTime.Sub(time.Now())
			prefix = formatDuration(remaining) + "> "
			useLivePrefix = remaining > 0
			return
		}),
		prompt.OptionHistory(historyEntries),
	).Run()
}

func formatDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}
